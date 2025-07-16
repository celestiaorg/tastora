package internal

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
	"os"
	"path/filepath"
	"sync"
)

var _ keyring.Keyring = &dockerKeyring{}

// dockerKeyring implements the keyring.Keyring interface by lazy loading from the provided docker container.
// Any writes to the keyring are also propagated to the container filesystem.
type dockerKeyring struct {
	mu                  sync.RWMutex
	dockerClient        *client.Client
	containerID         string
	containerKeyringDir string
	cdc                 codec.Codec

	// lazy-loaded keyring from container
	localKeyring keyring.Keyring
	tempDir      string
}

// NewDockerKeyring creates a new dockerKeyring instance.
func NewDockerKeyring(dockerClient *client.Client, containerID, containerKeyringDir string, cdc codec.Codec) keyring.Keyring {
	return &dockerKeyring{
		dockerClient:        dockerClient,
		containerID:         containerID,
		containerKeyringDir: containerKeyringDir,
		cdc:                 cdc,
	}
}

func (d *dockerKeyring) Backend() string {
	if err := d.ensureInitialized(); err != nil {
		return ""
	}
	return d.localKeyring.Backend()
}

func (d *dockerKeyring) List() ([]*keyring.Record, error) {
	if err := d.ensureInitialized(); err != nil {
		return nil, err
	}
	return d.localKeyring.List()
}

func (d *dockerKeyring) SupportedAlgorithms() (keyring.SigningAlgoList, keyring.SigningAlgoList) {
	if err := d.ensureInitialized(); err != nil {
		return keyring.SigningAlgoList{}, keyring.SigningAlgoList{}
	}
	return d.localKeyring.SupportedAlgorithms()
}

func (d *dockerKeyring) Key(uid string) (*keyring.Record, error) {
	if err := d.ensureInitialized(); err != nil {
		return nil, err
	}
	return d.localKeyring.Key(uid)
}

func (d *dockerKeyring) KeyByAddress(address sdk.Address) (*keyring.Record, error) {
	if err := d.ensureInitialized(); err != nil {
		return nil, err
	}
	return d.localKeyring.KeyByAddress(address)
}

func (d *dockerKeyring) Delete(uid string) error {
	if err := d.ensureInitialized(); err != nil {
		return err
	}

	if err := d.localKeyring.Delete(uid); err != nil {
		return fmt.Errorf("failed to delete key from local keyring: %w", err)
	}

	if err := d.deleteKeyFromContainer(uid); err != nil {
		return fmt.Errorf("failed to delete key from container: %w", err)
	}

	return nil
}

func (d *dockerKeyring) DeleteByAddress(address sdk.Address) error {
	if err := d.ensureInitialized(); err != nil {
		return err
	}

	record, err := d.localKeyring.KeyByAddress(address)
	if err != nil {
		return fmt.Errorf("failed to find key by address: %w", err)
	}

	if err := d.localKeyring.DeleteByAddress(address); err != nil {
		return fmt.Errorf("failed to delete key from local keyring: %w", err)
	}

	if err := d.deleteKeyFromContainer(record.Name); err != nil {
		return fmt.Errorf("failed to delete key from container: %w", err)
	}

	return nil
}

func (d *dockerKeyring) Rename(from, to string) error {
	if err := d.ensureInitialized(); err != nil {
		return err
	}

	if err := d.localKeyring.Rename(from, to); err != nil {
		return fmt.Errorf("failed to rename key in local keyring: %w", err)
	}

	if err := d.renameKeyInContainer(context.Background(), from, to); err != nil {
		return fmt.Errorf("failed to rename key in container: %w", err)
	}

	return nil
}

func (d *dockerKeyring) NewMnemonic(uid string, language keyring.Language, hdPath, bip39Passphrase string, algo keyring.SignatureAlgo) (*keyring.Record, string, error) {
	if err := d.ensureInitialized(); err != nil {
		return nil, "", err
	}

	// create new mnemonic in the local keyring
	record, mnemonic, err := d.localKeyring.NewMnemonic(uid, language, hdPath, bip39Passphrase, algo)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create new mnemonic: %w", err)
	}

	if err := d.persistKeyringToContainer(); err != nil {
		return nil, "", fmt.Errorf("failed to persist key to container: %w", err)
	}

	return record, mnemonic, nil
}

func (d *dockerKeyring) NewAccount(uid, mnemonic, bip39Passphrase, hdPath string, algo keyring.SignatureAlgo) (*keyring.Record, error) {
	if err := d.ensureInitialized(); err != nil {
		return nil, err
	}

	// Create new account in the in-memory keyring
	record, err := d.localKeyring.NewAccount(uid, mnemonic, bip39Passphrase, hdPath, algo)
	if err != nil {
		return nil, fmt.Errorf("failed to create new account: %w", err)
	}

	// Persist the new key to the Docker container
	if err := d.persistKeyringToContainer(); err != nil {
		return nil, fmt.Errorf("failed to persist key to container: %w", err)
	}

	return record, nil
}

func (d *dockerKeyring) SaveLedgerKey(uid string, algo keyring.SignatureAlgo, hrp string, coinType, account, index uint32) (*keyring.Record, error) {
	return nil, fmt.Errorf("ledger key saving not supported on docker keyring")
}

func (d *dockerKeyring) SaveOfflineKey(uid string, pubkey types.PubKey) (*keyring.Record, error) {
	if err := d.ensureInitialized(); err != nil {
		return nil, err
	}

	record, err := d.localKeyring.SaveOfflineKey(uid, pubkey)
	if err != nil {
		return nil, fmt.Errorf("failed to save offline key: %w", err)
	}

	if err := d.persistKeyringToContainer(); err != nil {
		return nil, fmt.Errorf("failed to persist key to container: %w", err)
	}

	return record, nil
}

func (d *dockerKeyring) SaveMultisig(uid string, pubkey types.PubKey) (*keyring.Record, error) {
	if err := d.ensureInitialized(); err != nil {
		return nil, err
	}

	// Save multisig key in the in-memory keyring
	record, err := d.localKeyring.SaveMultisig(uid, pubkey)
	if err != nil {
		return nil, fmt.Errorf("failed to save multisig key: %w", err)
	}

	if err := d.persistKeyringToContainer(); err != nil {
		return nil, fmt.Errorf("failed to persist key to container: %w", err)
	}

	return record, nil
}

func (d *dockerKeyring) Sign(uid string, msg []byte, signMode signing.SignMode) ([]byte, types.PubKey, error) {
	if err := d.ensureInitialized(); err != nil {
		return nil, nil, err
	}
	return d.localKeyring.Sign(uid, msg, signMode)
}

func (d *dockerKeyring) SignByAddress(address sdk.Address, msg []byte, signMode signing.SignMode) ([]byte, types.PubKey, error) {
	if err := d.ensureInitialized(); err != nil {
		return nil, nil, err
	}
	return d.localKeyring.SignByAddress(address, msg, signMode)
}

func (d *dockerKeyring) ImportPrivKey(uid, armor, passphrase string) error {
	if err := d.ensureInitialized(); err != nil {
		return err
	}

	if err := d.localKeyring.ImportPrivKey(uid, armor, passphrase); err != nil {
		return fmt.Errorf("failed to import private key: %w", err)
	}

	if err := d.persistKeyringToContainer(); err != nil {
		return fmt.Errorf("failed to persist key to container: %w", err)
	}

	return nil
}

func (d *dockerKeyring) ImportPrivKeyHex(uid, privKey, algoStr string) error {
	if err := d.ensureInitialized(); err != nil {
		return err
	}

	if err := d.localKeyring.ImportPrivKeyHex(uid, privKey, algoStr); err != nil {
		return fmt.Errorf("failed to import private key hex: %w", err)
	}

	if err := d.persistKeyringToContainer(); err != nil {
		return fmt.Errorf("failed to persist key to container: %w", err)
	}

	return nil
}

func (d *dockerKeyring) ImportPubKey(uid, armor string) error {
	if err := d.ensureInitialized(); err != nil {
		return err
	}
	return d.localKeyring.ImportPubKey(uid, armor)
}

func (d *dockerKeyring) ExportPubKeyArmor(uid string) (string, error) {
	if err := d.ensureInitialized(); err != nil {
		return "", err
	}
	return d.localKeyring.ExportPubKeyArmor(uid)
}

func (d *dockerKeyring) ExportPubKeyArmorByAddress(address sdk.Address) (string, error) {
	if err := d.ensureInitialized(); err != nil {
		return "", err
	}
	return d.localKeyring.ExportPubKeyArmorByAddress(address)
}

func (d *dockerKeyring) ExportPrivKeyArmor(uid, encryptPassphrase string) (armor string, err error) {
	if err := d.ensureInitialized(); err != nil {
		return "", err
	}
	return d.localKeyring.ExportPrivKeyArmor(uid, encryptPassphrase)
}

func (d *dockerKeyring) ExportPrivKeyArmorByAddress(address sdk.Address, encryptPassphrase string) (armor string, err error) {
	if err := d.ensureInitialized(); err != nil {
		return "", err
	}
	return d.localKeyring.ExportPrivKeyArmorByAddress(address, encryptPassphrase)
}

func (d *dockerKeyring) MigrateAll() ([]*keyring.Record, error) {
	if err := d.ensureInitialized(); err != nil {
		return nil, err
	}
	return d.localKeyring.MigrateAll()
}

// execCommand executes a command in the Docker container.
func (d *dockerKeyring) execCommand(ctx context.Context, cmd []string) error {
	execConfig := container.ExecOptions{
		Cmd: cmd,
	}

	exec, err := d.dockerClient.ContainerExecCreate(ctx, d.containerID, execConfig)
	if err != nil {
		return fmt.Errorf("failed to create exec: %w", err)
	}

	if err := d.dockerClient.ContainerExecStart(ctx, exec.ID, container.ExecStartOptions{}); err != nil {
		return fmt.Errorf("failed to execute command: %w", err)
	}

	return nil
}

// ensureInitialized lazily loads the keyring from the Docker container a temp directory for testing.
func (d *dockerKeyring) ensureInitialized() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.localKeyring != nil {
		return nil
	}

	// create temp directory for keyring files
	tempDir := filepath.Join(os.TempDir(), "docker-keyring-"+d.containerID)
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}

	d.tempDir = tempDir

	kr, err := NewLocalKeyringFromDockerContainer(
		context.TODO(), d.dockerClient, tempDir, d.containerKeyringDir, d.containerID)
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return fmt.Errorf("failed to create keyring from container: %w", err)
	}

	d.localKeyring = kr
	return nil
}

// createTarFromDirectory creates a tar archive from a directory
func createTarFromDirectory(srcDir string) (*bytes.Reader, error) {
	var tarBuf bytes.Buffer
	tarWriter := tar.NewWriter(&tarBuf)
	defer func() { _ = tarWriter.Close() }()

	err := filepath.Walk(srcDir, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(srcDir, filePath)
		if err != nil {
			return err
		}

		content, err := os.ReadFile(filePath)
		if err != nil {
			return err
		}

		header := &tar.Header{
			Name:     relPath,
			Mode:     int64(info.Mode()),
			Size:     int64(len(content)),
			ModTime:  info.ModTime(),
			Typeflag: tar.TypeReg,
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if _, err := tarWriter.Write(content); err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return bytes.NewReader(tarBuf.Bytes()), nil
}

// persistKeyringToContainer copies the entire temp keyring directory to the Docker container
func (d *dockerKeyring) persistKeyringToContainer() error {
	if d.tempDir == "" {
		return fmt.Errorf("temp directory not initialized")
	}

	keyringTestDir := filepath.Join(d.tempDir, "keyring-test")
	tarReader, err := createTarFromDirectory(keyringTestDir)
	if err != nil {
		return fmt.Errorf("failed to create tar from temp directory: %w", err)
	}

	// copy the entire tmp keyring directory to the container.
	if err := d.dockerClient.CopyToContainer(context.TODO(), d.containerID, d.containerKeyringDir, tarReader, container.CopyToContainerOptions{}); err != nil {
		return fmt.Errorf("failed to copy keyring directory to container: %w", err)
	}

	return nil
}

// deleteKeyFromContainer removes all key-related files from the Docker container
func (d *dockerKeyring) deleteKeyFromContainer(uid string) error {
	// delete main key file, .info file, and any .address files related to this key
	// use a glob pattern to match all files that might be related to this key
	keyPattern := filepath.Join(d.containerKeyringDir, uid+"*")

	if err := d.execCommand(context.TODO(), []string{"sh", "-c", fmt.Sprintf("rm -f %s", keyPattern)}); err != nil {
		return fmt.Errorf("failed to delete key files: %w", err)
	}

	// Also need to find and delete the .address file which is named by the address, not the uid
	// First get the key record to find its address, then delete the corresponding .address file
	record, err := d.localKeyring.Key(uid)
	if err == nil && record != nil {
		// Get the address and delete the corresponding .address file
		addr, err := record.GetAddress()
		if err == nil {
			addrFilePath := filepath.Join(d.containerKeyringDir, addr.String()+".address")
			_ = d.execCommand(context.TODO(), []string{"rm", "-f", addrFilePath})
		}
	}

	return nil
}

// renameKeyInContainer renames a key file in the Docker container
func (d *dockerKeyring) renameKeyInContainer(ctx context.Context, from, to string) error {
	fromPath := filepath.Join(d.containerKeyringDir, from)
	toPath := filepath.Join(d.containerKeyringDir, to)

	return d.execCommand(ctx, []string{"mv", fromPath, toPath})
}
