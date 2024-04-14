package github

import (
	"fmt"
	"net"
	"os"
	"path/filepath"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/helixml/helix/api/pkg/types"
	crypto_ssh "golang.org/x/crypto/ssh"
)

func CloneOrUpdateRepo(
	repo string,
	keypair types.KeyPair,
	repoPath string,
) error {
	if _, err := os.Stat(repoPath); err == nil {
		// Directory exists, pull the latest commits.
		repository, err := git.PlainOpen(repoPath)
		if err != nil {
			return fmt.Errorf("failed to open existing repo: %v", err)
		}

		worktree, err := repository.Worktree()
		if err != nil {
			return fmt.Errorf("failed to get worktree: %v", err)
		}

		err = worktree.Pull(&git.PullOptions{
			RemoteName: "origin",
			Auth:       makeAuth(keypair),
		})

		if err != nil && err != git.NoErrAlreadyUpToDate {
			return fmt.Errorf("failed to pull repo: %v", err)
		}
		return nil
	} else if os.IsNotExist(err) {
		// Directory does not exist, clone the repo.
		parentDir := filepath.Dir(repoPath)
		err := os.MkdirAll(parentDir, os.ModePerm)
		if err != nil {
			return fmt.Errorf("failed to create directory: %v", err)
		}
		_, err = git.PlainClone(repoPath, false, &git.CloneOptions{
			URL:      fmt.Sprintf("git@github.com:%s.git", repo),
			Progress: os.Stdout,
			Auth:     makeAuth(keypair),
		})
		if err != nil {
			return fmt.Errorf("failed to clone repo: %v", err)
		}
		return nil
	} else {
		return fmt.Errorf("failed to check if repo exists: %v", err)
	}
}

func GetRepoHash(
	repoPath string,
) (string, error) {
	if _, err := os.Stat(repoPath); err != nil {
		return "", err
	}
	repository, err := git.PlainOpen(repoPath)
	if err != nil {
		return "", fmt.Errorf("failed to open existing repo: %v", err)
	}
	ref, err := repository.Head()
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD: %v", err)
	}

	commit, err := repository.CommitObject(ref.Hash())
	if err != nil {
		return "", fmt.Errorf("failed to get commit object: %v", err)
	}

	return commit.Hash.String(), nil
}

func makeAuth(keypair types.KeyPair) transport.AuthMethod {
	signer, err := ssh.NewPublicKeys("git", []byte(keypair.PrivateKey), keypair.PublicKey)
	if err != nil {
		fmt.Println("Failed to create signer:", err)
		return nil
	}
	signer.HostKeyCallbackHelper = ssh.HostKeyCallbackHelper{
		HostKeyCallback: func(hostname string, remote net.Addr, key crypto_ssh.PublicKey) error {
			return nil
		},
	}
	return signer
}
