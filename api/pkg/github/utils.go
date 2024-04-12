package github

import (
	"fmt"
	"os"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/helixml/helix/api/pkg/types"
)

func cloneOrUpdateRepo(
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
		_, err := git.PlainClone(repoPath, false, &git.CloneOptions{
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

func makeAuth(keypair types.KeyPair) transport.AuthMethod {
	signer, err := ssh.NewPublicKeys("git", []byte(keypair.PrivateKey), keypair.PublicKey)
	if err != nil {
		fmt.Println("Failed to create signer:", err)
		return nil
	}
	return signer
}

// func main() {
// 	// Example usage:
// 	keypair := Keypair{
// 		PrivateKey: "YOUR_PRIVATE_KEY_HERE",
// 		PublicKey:  "",
// 	}
// 	repoPath, err := CloneOrUpdateRepo("binocarlos/puta", keypair, "/path/to/base/folder")
// 	if err != nil {
// 		fmt.Println("Error:", err)
// 		return
// 	}
// 	fmt.Println("Repository cloned or updated at:", repoPath)
// }
