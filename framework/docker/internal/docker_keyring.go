package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	"github.com/docker/docker/api/types/container"
	"github.com/moby/moby/client"
	"io"
	"path"
	"path/filepath"
	"sync"
	"time"
)

var _ keyring.Keyring = &dockerKeyring{}

type dockerKeyring struct {
	mu                  sync.RWMutex
	dockerClient        *client.Client
	containerID         string
	containerKeyringDir string
	cdc                 codec.Codec

	// Lazy-loaded keyring from container
	localKeyring keyring.Keyring
	tempDir      string
}

// NewDockerKeyring creates a new keyring that proxies to Docker container keys
func NewDockerKeyring(dockerClient *client.Client, containerID, containerKeyringDir string, cdc codec.Codec) keyring.Keyring {
	return &dockerKeyring{
		dockerClient:        dockerClient,
		containerID:         containerID,
		containerKeyringDir: containerKeyringDir,
		cdc:                 cdc,
	}
}

// ensureInitialized lazily loads the keyring from the Docker container into memory
func (d *dockerKeyring) ensureInitialized(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.localKeyring != nil {
		return nil
	}

	kr := keyring.NewInMemory(d.cdc)

	// copy the contents of the keyring directory from the docker container.
	reader, _, err := d.dockerClient.CopyFromContainer(ctx, d.containerID, d.containerKeyringDir)
	if err != nil {
		return fmt.Errorf("failed to copy keyring from container: %w", err)
	}
	defer reader.Close()

	// Extract and parse keyring files
	tr := tar.NewReader(reader)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar entry: %w", err)
		}

		if hdr.Typeflag != tar.TypeReg {
			continue
		}

		var fileBuff bytes.Buffer
		if _, err := io.Copy(&fileBuff, tr); err != nil {
			return fmt.Errorf("failed to read file content: %w", err)
		}

		fileName := path.Base(hdr.Name)
		if fileName != "" {
			// import each key file into the in-memory keyring
			if err := d.importKeyFromFile(kr, fileName, fileBuff.Bytes()); err != nil {
				return fmt.Errorf("failed to import key file %s: %w", fileName, err)
			}
		}
	}

	d.localKeyring = kr
	return nil
}

// importKeyFromFile parses a keyring file and imports the key into the in-memory keyring
func (d *dockerKeyring) importKeyFromFile(kr keyring.Keyring, fileName string, content []byte) error {
	// For test keyring backend, keys are stored as individual files
	// The filename is typically the key name, and content is the key data

	// Try to import as armored private key (common format)
	if err := kr.ImportPrivKey(fileName, string(content), ""); err != nil {
		return fmt.Errorf("unsupported key format in file %s: %w", fileName, err)
	}

	return nil
}

// persistKeyToContainer exports a key from the in-memory keyring and writes it to the Docker container
func (d *dockerKeyring) persistKeyToContainer(ctx context.Context, uid string) error {
	// Export the private key as armor
	armor, err := d.localKeyring.ExportPrivKeyArmor(uid, "")
	if err != nil {
		return fmt.Errorf("failed to export private key: %w", err)
	}

	// Create a tar archive with the key file
	var tarBuf bytes.Buffer
	tarWriter := tar.NewWriter(&tarBuf)

	// Add the key file to the tar archive
	keyFileName := uid // Use the UID as the filename
	header := &tar.Header{
		Name:     keyFileName,
		Mode:     0600,
		Size:     int64(len(armor)),
		ModTime:  time.Now(),
		Typeflag: tar.TypeReg,
	}

	if err := tarWriter.WriteHeader(header); err != nil {
		return fmt.Errorf("failed to write tar header: %w", err)
	}

	if _, err := tarWriter.Write([]byte(armor)); err != nil {
		return fmt.Errorf("failed to write key data to tar: %w", err)
	}

	if err := tarWriter.Close(); err != nil {
		return fmt.Errorf("failed to close tar writer: %w", err)
	}

	// Copy the tar archive to the container's keyring directory
	if err := d.dockerClient.CopyToContainer(ctx, d.containerID, d.containerKeyringDir, &tarBuf, container.CopyToContainerOptions{}); err != nil {
		return fmt.Errorf("failed to copy key to container: %w", err)
	}

	return nil
}

// deleteKeyFromContainer removes a key file from the Docker container
func (d *dockerKeyring) deleteKeyFromContainer(ctx context.Context, uid string) error {
	// Execute rm command in the container to delete the key file
	keyFilePath := filepath.Join(d.containerKeyringDir, uid)

	execConfig := container.ExecOptions{
		Cmd: []string{"rm", "-f", keyFilePath},
	}

	exec, err := d.dockerClient.ContainerExecCreate(ctx, d.containerID, execConfig)
	if err != nil {
		return fmt.Errorf("failed to create exec for key deletion: %w", err)
	}

	if err := d.dockerClient.ContainerExecStart(ctx, exec.ID, container.ExecStartOptions{}); err != nil {
		return fmt.Errorf("failed to execute key deletion: %w", err)
	}

	return nil
}

// renameKeyInContainer renames a key file in the Docker container
func (d *dockerKeyring) renameKeyInContainer(ctx context.Context, from, to string) error {
	// Execute mv command in the container to rename the key file
	fromPath := filepath.Join(d.containerKeyringDir, from)
	toPath := filepath.Join(d.containerKeyringDir, to)

	execConfig := container.ExecOptions{
		Cmd: []string{"mv", fromPath, toPath},
	}

	exec, err := d.dockerClient.ContainerExecCreate(ctx, d.containerID, execConfig)
	if err != nil {
		return fmt.Errorf("failed to create exec for key rename: %w", err)
	}

	if err := d.dockerClient.ContainerExecStart(ctx, exec.ID, container.ExecStartOptions{}); err != nil {
		return fmt.Errorf("failed to execute key rename: %w", err)
	}

	return nil
}

func (d *dockerKeyring) Backend() string {
	if err := d.ensureInitialized(context.Background()); err != nil {
		return ""
	}
	return d.localKeyring.Backend()
}

func (d *dockerKeyring) List() ([]*keyring.Record, error) {
	if err := d.ensureInitialized(context.Background()); err != nil {
		return nil, err
	}
	return d.localKeyring.List()
}

func (d *dockerKeyring) SupportedAlgorithms() (keyring.SigningAlgoList, keyring.SigningAlgoList) {
	if err := d.ensureInitialized(context.Background()); err != nil {
		// Return default algorithms if initialization fails
		return keyring.SigningAlgoList{}, keyring.SigningAlgoList{}
	}
	return d.localKeyring.SupportedAlgorithms()
}

func (d *dockerKeyring) Key(uid string) (*keyring.Record, error) {
	if err := d.ensureInitialized(context.Background()); err != nil {
		return nil, err
	}
	return d.localKeyring.Key(uid)
}

func (d *dockerKeyring) KeyByAddress(address sdk.Address) (*keyring.Record, error) {
	if err := d.ensureInitialized(context.Background()); err != nil {
		return nil, err
	}
	return d.localKeyring.KeyByAddress(address)
}

func (d *dockerKeyring) Delete(uid string) error {
	if err := d.ensureInitialized(context.Background()); err != nil {
		return err
	}

	// Delete from in-memory keyring
	if err := d.localKeyring.Delete(uid); err != nil {
		return fmt.Errorf("failed to delete key from local keyring: %w", err)
	}

	// Delete from Docker container
	if err := d.deleteKeyFromContainer(context.Background(), uid); err != nil {
		return fmt.Errorf("failed to delete key from container: %w", err)
	}

	return nil
}

func (d *dockerKeyring) DeleteByAddress(address sdk.Address) error {
	if err := d.ensureInitialized(context.Background()); err != nil {
		return err
	}

	// First get the key record to find the UID
	record, err := d.localKeyring.KeyByAddress(address)
	if err != nil {
		return fmt.Errorf("failed to find key by address: %w", err)
	}

	// Delete from in-memory keyring
	if err := d.localKeyring.DeleteByAddress(address); err != nil {
		return fmt.Errorf("failed to delete key from local keyring: %w", err)
	}

	// Delete from Docker container using the UID
	if err := d.deleteKeyFromContainer(context.Background(), record.Name); err != nil {
		return fmt.Errorf("failed to delete key from container: %w", err)
	}

	return nil
}

func (d *dockerKeyring) Rename(from, to string) error {
	if err := d.ensureInitialized(context.Background()); err != nil {
		return err
	}

	// Rename in in-memory keyring
	if err := d.localKeyring.Rename(from, to); err != nil {
		return fmt.Errorf("failed to rename key in local keyring: %w", err)
	}

	// Rename in Docker container
	if err := d.renameKeyInContainer(context.Background(), from, to); err != nil {
		return fmt.Errorf("failed to rename key in container: %w", err)
	}

	return nil
}

func (d *dockerKeyring) NewMnemonic(uid string, language keyring.Language, hdPath, bip39Passphrase string, algo keyring.SignatureAlgo) (*keyring.Record, string, error) {
	if err := d.ensureInitialized(context.Background()); err != nil {
		return nil, "", err
	}

	// Create new mnemonic in the in-memory keyring
	record, mnemonic, err := d.localKeyring.NewMnemonic(uid, language, hdPath, bip39Passphrase, algo)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create new mnemonic: %w", err)
	}

	// Persist the new key to the Docker container
	if err := d.persistKeyToContainer(context.Background(), uid); err != nil {
		return nil, "", fmt.Errorf("failed to persist key to container: %w", err)
	}

	return record, mnemonic, nil
}

func (d *dockerKeyring) NewAccount(uid, mnemonic, bip39Passphrase, hdPath string, algo keyring.SignatureAlgo) (*keyring.Record, error) {
	if err := d.ensureInitialized(context.Background()); err != nil {
		return nil, err
	}

	// Create new account in the in-memory keyring
	record, err := d.localKeyring.NewAccount(uid, mnemonic, bip39Passphrase, hdPath, algo)
	if err != nil {
		return nil, fmt.Errorf("failed to create new account: %w", err)
	}

	// Persist the new key to the Docker container
	if err := d.persistKeyToContainer(context.Background(), uid); err != nil {
		return nil, fmt.Errorf("failed to persist key to container: %w", err)
	}

	return record, nil
}

func (d *dockerKeyring) SaveLedgerKey(uid string, algo keyring.SignatureAlgo, hrp string, coinType, account, index uint32) (*keyring.Record, error) {
	return nil, fmt.Errorf("ledger key saving not supported on docker keyring")
}

func (d *dockerKeyring) SaveOfflineKey(uid string, pubkey types.PubKey) (*keyring.Record, error) {
	if err := d.ensureInitialized(context.Background()); err != nil {
		return nil, err
	}

	// Save offline key in the in-memory keyring
	record, err := d.localKeyring.SaveOfflineKey(uid, pubkey)
	if err != nil {
		return nil, fmt.Errorf("failed to save offline key: %w", err)
	}

	// Persist the key to the Docker container
	if err := d.persistKeyToContainer(context.Background(), uid); err != nil {
		return nil, fmt.Errorf("failed to persist key to container: %w", err)
	}

	return record, nil
}

func (d *dockerKeyring) SaveMultisig(uid string, pubkey types.PubKey) (*keyring.Record, error) {
	if err := d.ensureInitialized(context.Background()); err != nil {
		return nil, err
	}

	// Save multisig key in the in-memory keyring
	record, err := d.localKeyring.SaveMultisig(uid, pubkey)
	if err != nil {
		return nil, fmt.Errorf("failed to save multisig key: %w", err)
	}

	// Persist the key to the Docker container
	if err := d.persistKeyToContainer(context.Background(), uid); err != nil {
		return nil, fmt.Errorf("failed to persist key to container: %w", err)
	}

	return record, nil
}

func (d *dockerKeyring) Sign(uid string, msg []byte, signMode signing.SignMode) ([]byte, types.PubKey, error) {
	if err := d.ensureInitialized(context.Background()); err != nil {
		return nil, nil, err
	}
	return d.localKeyring.Sign(uid, msg, signMode)
}

func (d *dockerKeyring) SignByAddress(address sdk.Address, msg []byte, signMode signing.SignMode) ([]byte, types.PubKey, error) {
	if err := d.ensureInitialized(context.Background()); err != nil {
		return nil, nil, err
	}
	return d.localKeyring.SignByAddress(address, msg, signMode)
}

func (d *dockerKeyring) ImportPrivKey(uid, armor, passphrase string) error {
	if err := d.ensureInitialized(context.Background()); err != nil {
		return err
	}

	// Import private key into the in-memory keyring
	if err := d.localKeyring.ImportPrivKey(uid, armor, passphrase); err != nil {
		return fmt.Errorf("failed to import private key: %w", err)
	}

	// Persist the key to the Docker container
	if err := d.persistKeyToContainer(context.Background(), uid); err != nil {
		return fmt.Errorf("failed to persist key to container: %w", err)
	}

	return nil
}

func (d *dockerKeyring) ImportPrivKeyHex(uid, privKey, algoStr string) error {
	if err := d.ensureInitialized(context.Background()); err != nil {
		return err
	}

	// Import private key hex into the in-memory keyring
	if err := d.localKeyring.ImportPrivKeyHex(uid, privKey, algoStr); err != nil {
		return fmt.Errorf("failed to import private key hex: %w", err)
	}

	// Persist the key to the Docker container
	if err := d.persistKeyToContainer(context.Background(), uid); err != nil {
		return fmt.Errorf("failed to persist key to container: %w", err)
	}

	return nil
}

func (d *dockerKeyring) ImportPubKey(uid, armor string) error {
	if err := d.ensureInitialized(context.Background()); err != nil {
		return err
	}
	return d.localKeyring.ImportPubKey(uid, armor)
}

func (d *dockerKeyring) ExportPubKeyArmor(uid string) (string, error) {
	if err := d.ensureInitialized(context.Background()); err != nil {
		return "", err
	}
	return d.localKeyring.ExportPubKeyArmor(uid)
}

func (d *dockerKeyring) ExportPubKeyArmorByAddress(address sdk.Address) (string, error) {
	if err := d.ensureInitialized(context.Background()); err != nil {
		return "", err
	}
	return d.localKeyring.ExportPubKeyArmorByAddress(address)
}

func (d *dockerKeyring) ExportPrivKeyArmor(uid, encryptPassphrase string) (armor string, err error) {
	if err := d.ensureInitialized(context.Background()); err != nil {
		return "", err
	}
	return d.localKeyring.ExportPrivKeyArmor(uid, encryptPassphrase)
}

func (d *dockerKeyring) ExportPrivKeyArmorByAddress(address sdk.Address, encryptPassphrase string) (armor string, err error) {
	if err := d.ensureInitialized(context.Background()); err != nil {
		return "", err
	}
	return d.localKeyring.ExportPrivKeyArmorByAddress(address, encryptPassphrase)
}

func (d *dockerKeyring) MigrateAll() ([]*keyring.Record, error) {
	if err := d.ensureInitialized(context.Background()); err != nil {
		return nil, err
	}
	return d.localKeyring.MigrateAll()
}
