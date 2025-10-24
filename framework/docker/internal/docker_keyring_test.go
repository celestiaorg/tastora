package internal

import (
	"context"
	tastoraclient "github.com/celestiaorg/tastora/framework/docker/client"
	"io"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/suite"
)

type DockerKeyringTestSuite struct {
	suite.Suite
	dockerClient *tastoraclient.Client
	containerID  string
	keyringDir   string
	cdc          codec.Codec
	kr           keyring.Keyring
}

func TestDockerKeyringTestSuite(t *testing.T) {
	suite.Run(t, new(DockerKeyringTestSuite))
}

func (s *DockerKeyringTestSuite) SetupSuite() {
	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	s.Require().NoError(err)

	s.dockerClient = tastoraclient.NewClient(dockerClient, "docker-keyring-test-suite")

	registry := codectypes.NewInterfaceRegistry()
	cryptocodec.RegisterInterfaces(registry)
	s.cdc = codec.NewProtoCodec(registry)

	// pull a minimal image for testing
	ctx := context.Background()
	pullReader, err := s.dockerClient.ImagePull(ctx, "alpine:latest", image.PullOptions{})
	s.Require().NoError(err)

	_, err = io.Copy(io.Discard, pullReader)
	_ = pullReader.Close()
	s.Require().NoError(err)

	// create a test container
	containerConfig := &container.Config{
		Image: "alpine:latest",
		Cmd:   []string{"sleep", "3600"}, // keep container running
	}

	resp, err := s.dockerClient.ContainerCreate(ctx, containerConfig, nil, nil, nil, "")
	s.Require().NoError(err)
	s.containerID = resp.ID

	err = s.dockerClient.ContainerStart(ctx, s.containerID, container.StartOptions{})
	s.Require().NoError(err)

	// wait for container to be ready
	time.Sleep(time.Second)

	// wet up keyring directory in container
	s.keyringDir = "/tmp/keyring-test"

	// create keyring directory in container
	execConfig := container.ExecOptions{
		Cmd: []string{"mkdir", "-p", s.keyringDir},
	}
	exec, err := s.dockerClient.ContainerExecCreate(ctx, s.containerID, execConfig)
	s.Require().NoError(err)

	err = s.dockerClient.ContainerExecStart(ctx, exec.ID, container.ExecStartOptions{})
	s.Require().NoError(err)

	// create the docker keyring
	s.kr = NewDockerKeyring(s.dockerClient, s.containerID, s.keyringDir, s.cdc)
}

func (s *DockerKeyringTestSuite) TearDownSuite() {
	if s.containerID != "" {
		ctx := context.Background()
		_ = s.dockerClient.ContainerStop(ctx, s.containerID, container.StopOptions{})
		_ = s.dockerClient.ContainerRemove(ctx, s.containerID, container.RemoveOptions{})
	}
}

func (s *DockerKeyringTestSuite) TestBackend() {
	backend := s.kr.Backend()
	s.Require().NotEmpty(backend)
}

func (s *DockerKeyringTestSuite) TestNewMnemonic() {
	// Create a new mnemonic
	record, mnemonic, err := s.kr.NewMnemonic(
		"test-key",
		keyring.English,
		"m/44'/118'/0'/0/0",
		"",
		hd.Secp256k1,
	)

	s.Require().NoError(err)
	s.Require().NotNil(record)
	s.Require().NotEmpty(mnemonic)
	s.Require().Equal("test-key", record.Name)

	// verify the key exists in the keyring
	retrievedRecord, err := s.kr.Key("test-key")
	s.Require().NoError(err)
	s.Require().Equal(record.Name, retrievedRecord.Name)

	// verify the key was persisted to container by creating a new keyring instance
	newKr := NewDockerKeyring(s.dockerClient, s.containerID, s.keyringDir, s.cdc)
	persistedRecord, err := newKr.Key("test-key")
	s.Require().NoError(err)
	s.Require().Equal(record.Name, persistedRecord.Name)
}

func (s *DockerKeyringTestSuite) TestNewAccount() {
	mnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

	record, err := s.kr.NewAccount(
		"test-account",
		mnemonic,
		"",
		"m/44'/118'/0'/0/0",
		hd.Secp256k1,
	)

	s.Require().NoError(err)
	s.Require().NotNil(record)
	s.Require().Equal("test-account", record.Name)

	// verify the key exists
	retrievedRecord, err := s.kr.Key("test-account")
	s.Require().NoError(err)
	s.Require().Equal(record.Name, retrievedRecord.Name)
}

func (s *DockerKeyringTestSuite) TestList() {
	// create a few test keys
	_, _, err := s.kr.NewMnemonic("key1", keyring.English, "m/44'/118'/0'/0/0", "", hd.Secp256k1)
	s.Require().NoError(err)

	_, _, err = s.kr.NewMnemonic("key2", keyring.English, "m/44'/118'/0'/0/1", "", hd.Secp256k1)
	s.Require().NoError(err)

	// list all keys
	records, err := s.kr.List()
	s.Require().NoError(err)
	s.Require().GreaterOrEqual(len(records), 2)

	// Check that our keys are in the list
	keyNames := make(map[string]bool)
	for _, record := range records {
		keyNames[record.Name] = true
	}
	s.Require().True(keyNames["key1"])
	s.Require().True(keyNames["key2"])
}

func (s *DockerKeyringTestSuite) TestSign() {
	// Create a test key
	record, _, err := s.kr.NewMnemonic("sign-test", keyring.English, "m/44'/118'/0'/0/0", "", hd.Secp256k1)
	s.Require().NoError(err)
	// Test message to sign
	message := []byte("test message to sign")

	// sign the message
	signature, pubKey, err := s.kr.Sign("sign-test", message, signing.SignMode_SIGN_MODE_DIRECT)
	s.Require().NoError(err)
	s.Require().NotEmpty(signature)
	s.Require().NotNil(pubKey)

	// verify signature matches the record's public key
	recordPubKey, err := record.GetPubKey()
	s.Require().NoError(err)
	s.Require().True(pubKey.Equals(recordPubKey))
}

func (s *DockerKeyringTestSuite) TestDelete() {
	_, _, err := s.kr.NewMnemonic("delete-test", keyring.English, "m/44'/118'/0'/0/0", "", hd.Secp256k1)
	s.Require().NoError(err)

	// verify the key exists
	_, err = s.kr.Key("delete-test")
	s.Require().NoError(err)

	// delete the key
	err = s.kr.Delete("delete-test")
	s.Require().NoError(err)

	// verify the key no longer exists
	_, err = s.kr.Key("delete-test")
	s.Require().Error(err)

	// verify the key was deleted from container by creating a new keyring instance
	newKr := NewDockerKeyring(s.dockerClient, s.containerID, s.keyringDir, s.cdc)
	_, err = newKr.Key("delete-test")
	s.Require().Error(err)
}

func (s *DockerKeyringTestSuite) TestDeleteByAddress() {
	record, _, err := s.kr.NewMnemonic("delete-by-addr-test", keyring.English, "m/44'/118'/0'/0/0", "", hd.Secp256k1)
	s.Require().NoError(err, "failed to create key")

	addr, err := record.GetAddress()
	s.Require().NoError(err, "failed to get address")

	_, err = s.kr.KeyByAddress(addr)
	s.Require().NoError(err, "failed to get key by address")

	err = s.kr.DeleteByAddress(addr)
	s.Require().NoError(err, "failed to delete key by address")

	_, err = s.kr.KeyByAddress(addr)
	s.Require().Error(err, "expected error when getting key by address after deletion")
}

func (s *DockerKeyringTestSuite) TestRename() {
	originalRecord, _, err := s.kr.NewMnemonic("original-name", keyring.English, "m/44'/118'/0'/0/0", "", hd.Secp256k1)
	s.Require().NoError(err, "failed to create key")

	_, err = s.kr.Key("original-name")
	s.Require().NoError(err, "failed to get key")

	err = s.kr.Rename("original-name", "new-name")
	s.Require().NoError(err, "failed to rename key")

	renamedRecord, err := s.kr.Key("new-name")
	s.Require().NoError(err, "failed to get renamed key")
	s.Require().Equal("new-name", renamedRecord.Name)

	_, err = s.kr.Key("original-name")
	s.Require().Error(err, "expected error when getting original key after renaming")

	originalPubKey, err := originalRecord.GetPubKey()
	s.Require().NoError(err, "failed to get original key's pubkey")
	renamedPubKey, err := renamedRecord.GetPubKey()
	s.Require().NoError(err, "failed to get renamed key's pubkey")
	s.Require().True(originalPubKey.Equals(renamedPubKey))

	// verify persistence by creating a new keyring instance
	newKr := NewDockerKeyring(s.dockerClient, s.containerID, s.keyringDir, s.cdc)
	persistedRecord, err := newKr.Key("new-name")
	s.Require().NoError(err, "failed to get renamed key from new keyring")
	s.Require().Equal("new-name", persistedRecord.Name)
}

func (s *DockerKeyringTestSuite) TestImportPrivKey() {
	// first create a key to get its armor
	record, _, err := s.kr.NewMnemonic("export-test", keyring.English, "m/44'/118'/0'/0/0", "", hd.Secp256k1)
	s.Require().NoError(err, "failed to create key")

	armor, err := s.kr.ExportPrivKeyArmor("export-test", "test-password")
	s.Require().NoError(err, "failed to export key")
	s.Require().NotEmpty(armor, "exported key is empty")

	err = s.kr.Delete("export-test")
	s.Require().NoError(err, "failed to delete key")

	// import the key with a new name
	err = s.kr.ImportPrivKey("import-test", armor, "test-password")
	s.Require().NoError(err, "failed to import key")

	// verify the imported key works
	importedRecord, err := s.kr.Key("import-test")
	s.Require().NoError(err, "failed to get imported key")
	s.Require().Equal("import-test", importedRecord.Name, "imported key has wrong name")

	// verify the public keys match
	originalPubKey, err := record.GetPubKey()
	s.Require().NoError(err)
	importedPubKey, err := importedRecord.GetPubKey()
	s.Require().NoError(err)
	s.Require().True(originalPubKey.Equals(importedPubKey))
}

func (s *DockerKeyringTestSuite) TestExportPubKeyArmor() {
	_, _, err := s.kr.NewMnemonic("export-pub-test", keyring.English, "m/44'/118'/0'/0/0", "", hd.Secp256k1)
	s.Require().NoError(err)

	armor, err := s.kr.ExportPubKeyArmor("export-pub-test")
	s.Require().NoError(err)
	s.Require().NotEmpty(armor)
	s.Require().Contains(armor, "-----BEGIN")
	s.Require().Contains(armor, "-----END")
}

func (s *DockerKeyringTestSuite) TestSupportedAlgorithms() {
	supported, _ := s.kr.SupportedAlgorithms()
	s.Require().NotEmpty(supported)
}

func (s *DockerKeyringTestSuite) TestPersistenceAcrossInstances() {
	// create a key with the first keyring instance
	originalRecord, _, err := s.kr.NewMnemonic("persistence-test", keyring.English, "m/44'/118'/0'/0/0", "", hd.Secp256k1)
	s.Require().NoError(err)

	// create a new keyring instance pointing to the same container
	newKr := NewDockerKeyring(s.dockerClient, s.containerID, s.keyringDir, s.cdc)

	// verify the key exists in the new instance
	persistedRecord, err := newKr.Key("persistence-test")
	s.Require().NoError(err)
	s.Require().Equal(originalRecord.Name, persistedRecord.Name)

	// verify the public keys match
	originalPubKey, err := originalRecord.GetPubKey()
	s.Require().NoError(err)
	persistedPubKey, err := persistedRecord.GetPubKey()
	s.Require().NoError(err)
	s.Require().True(originalPubKey.Equals(persistedPubKey))
}
