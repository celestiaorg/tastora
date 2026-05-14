package internal

import (
	"archive/tar"
	"bytes"
	"context"
	"github.com/celestiaorg/tastora/framework/types"
	"io"
	"os"
	"path"
	"path/filepath"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/moby/moby/client"
)

// NewLocalKeyringFromDockerContainer copies the contents of the given container directory into a specified local directory.
// This allows test hosts to sign transactions on behalf of test users.
func NewLocalKeyringFromDockerContainer(ctx context.Context, dc types.TastoraDockerClient, localDirectory, containerKeyringDir, containerID string) (keyring.Keyring, error) {
	copyResult, err := dc.CopyFromContainer(ctx, containerID, client.CopyFromContainerOptions{
		SourcePath: containerKeyringDir,
	})
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Join(localDirectory, "keyring-test"), os.ModePerm); err != nil {
		return nil, err
	}
	reader := copyResult.Content
	defer func() {
		_ = reader.Close()
	}()
	tr := tar.NewReader(reader)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			return nil, err
		}

		var fileBuff bytes.Buffer
		if _, err := io.Copy(&fileBuff, tr); err != nil { //nolint: gosec
			return nil, err
		}

		name := hdr.Name
		extractedFileName := path.Base(name)
		isDirectory := extractedFileName == ""
		if isDirectory {
			continue
		}

		filePath := filepath.Join(localDirectory, "keyring-test", extractedFileName)
		if err := os.WriteFile(filePath, fileBuff.Bytes(), os.ModePerm); err != nil {
			return nil, err
		}
	}

	registry := codectypes.NewInterfaceRegistry()
	cryptocodec.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)

	return keyring.New("", keyring.BackendTest, localDirectory, os.Stdin, cdc)
}
