package service

import (
	"context"
	"fmt"
	"path"

	dbEngine "github.com/flotio-dev/core-api/internal/common/database"
	githubEngine "github.com/flotio-dev/core-api/internal/infra/github"
	repositories "github.com/flotio-dev/core-api/internal/modules/github/repository"
	"github.com/google/go-github/v79/github"
)

type GithubService struct {
	Repository    *repositories.GithubRepository
	ClientManager *githubEngine.GitHubClientManager
}

func NewGithubService(repository *repositories.GithubRepository, ClientManager *githubEngine.GitHubClientManager) *GithubService {
	return &GithubService{
		Repository:    repository,
		ClientManager: ClientManager,
	}
}

func (s *GithubService) SaveInstallation(userID uint, installationID int64, accountLogin, accountType string, targetID int64) error {
	return s.Repository.SaveInstallation(userID, installationID, accountLogin, accountType, targetID)
}

func (s *GithubService) GetInstallationByUser(userID uint) (*dbEngine.GithubInstallation, error) {
	return s.Repository.GetInstallationByUser(userID)
}

func (s *GithubService) ListRepositories(ctx context.Context, installationID int64) ([]*github.Repository, error) {
	client, err := s.ClientManager.ClientForInstallation(installationID)
	if err != nil {
		return nil, fmt.Errorf("cannot create github client: %w", err)
	}

	reposResp, _, err := client.Apps.ListRepos(ctx, &github.ListOptions{PerPage: 100})
	if err != nil {
		return nil, fmt.Errorf("github api error: %w", err)
	}

	return reposResp.Repositories, nil
}

func (s *GithubService) GetRepoTree(ctx context.Context, installationID int64, owner, repo string) ([]*github.RepositoryContent, error) {
	client, err := s.ClientManager.ClientForInstallation(installationID)
	if err != nil {
		return nil, err
	}

	var result []*github.RepositoryContent

	var fetch func(path string) error
	fetch = func(path string) error {
		_, contents, _, err := client.Repositories.GetContents(ctx, owner, repo, path, nil)
		if err != nil {
			return err
		}

		for _, c := range contents {
			if c.GetType() == "dir" {
				result = append(result, c)
				if err := fetch(c.GetPath()); err != nil {
					return err
				}
			}
		}
		return nil
	}

	if err := fetch(""); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *GithubService) GetInstallationToken(installationID int64) (string, error) {
	client, err := s.ClientManager.ClientForApp()
	if err != nil {
		return "", err
	}

	tokenResp, _, err := client.Apps.CreateInstallationToken(context.Background(), installationID, nil)
	if err != nil {
		return "", err
	}

	return tokenResp.GetToken(), nil
}

func (s *GithubService) GetGithubUser(ctx context.Context, installationID int64) (*github.User, error) {
	client, err := s.ClientManager.ClientForInstallation(installationID)
	if err != nil {
		return nil, err
	}

	user, _, err := client.Users.Get(ctx, "")
	if err != nil {
		return nil, err
	}

	return user, nil
}

func (s *GithubService) GetGithubInstallation(ctx context.Context, installationID int64) (*github.Installation, error) {
	fmt.Printf("client ID = %d\n", s.ClientManager.AppID)
	fmt.Printf("privateKeyPath = %s\n", s.ClientManager.PrivateKeyPath)

	client, err := s.ClientManager.ClientForApp()
	if err != nil {
		fmt.Printf("Error creating GitHub client: %v\n", err)
		return nil, err
	}

	inst, resp, err := client.Apps.GetInstallation(ctx, installationID)
	if err != nil {
		fmt.Printf("Error fetching GitHub installation %d: %v\n", installationID, err)
		fmt.Printf("Response status: %d\n", resp.StatusCode)
		fmt.Printf("Response body: %s\n", resp.Body)
		return nil, err
	}

	return inst, nil
}

func (s *GithubService) InstallationExists(ctx context.Context, installationID int64) (*github.Installation, error) {
	client, err := s.ClientManager.ClientForApp()
	if err != nil {
		return nil, err
	}

	inst, resp, err := client.Apps.GetInstallation(ctx, installationID)
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			return nil, nil
		}
		return nil, err
	}
	return inst, nil
}

func (s *GithubService) GetGithubInstallationByInstallationID(installationID int64) (*dbEngine.GithubInstallation, error) {
	return s.Repository.GetGithubInstallationByInstallationID(installationID)
}

func (s *GithubService) DeleteInstallationByInstallationID(installationID int64) error {
	return s.Repository.DeleteInstallationByInstallationID(installationID)
}

// DeleteInstallation supprime l'installation côté GitHub (si possible) puis supprime l'enregistrement en base.
// Si l'appel GitHub renvoie 404, on continue et supprime uniquement l'enregistrement DB.
func (s *GithubService) DeleteInstallation(ctx context.Context, installationID int64) error {
	// tenter suppression côté GitHub avec le client App
	client, err := s.ClientManager.ClientForApp()
	if err == nil {
		// essayer de supprimer l'installation sur GitHub ; certains comptes peuvent ne pas autoriser
		resp, derr := client.Apps.DeleteInstallation(ctx, installationID)
		if derr != nil {
			if resp != nil && resp.StatusCode != 404 {
				// erreur non-404 -> considérer comme bloquante
				return derr
			}
			// si 404, on ignore et on continue vers suppression DB
		}
	} else {
		// si on ne peut pas créer de client App, renvoyer l'erreur
		return err
	}

	// suppression en base
	if err := s.Repository.DeleteInstallationByInstallationID(installationID); err != nil {
		return err
	}

	return nil
}

func (s *GithubService) FindBuildPath(ctx context.Context, installationID int64, owner, repo string) (string, error) {
	client, err := s.ClientManager.ClientForInstallation(installationID)
	if err != nil {
		return "", fmt.Errorf("cannot create github client: %w", err)
	}

	var find func(p string) (string, error)
	find = func(p string) (string, error) {
		_, contents, _, err := client.Repositories.GetContents(ctx, owner, repo, p, nil)
		if err != nil {
			return "", err
		}
		for _, c := range contents {
			if c.GetType() == "file" && c.GetName() == "pubspec.yaml" {
				dir := path.Dir(c.GetPath())
				if dir == "." {
					dir = ""
				}
				return dir, nil
			}
			if c.GetType() == "dir" {
				if d, err := find(c.GetPath()); err == nil && d != "" {
					return d, nil
				} else if err == nil && d == "" {
					return d, nil
				}
			}
		}
		return "", nil
	}

	dir, err := find("")
	if err != nil {
		return "", err
	}
	if dir == "" {
		_, contents, _, cerr := client.Repositories.GetContents(ctx, owner, repo, "", nil)
		if cerr != nil {
			return "", cerr
		}
		for _, c := range contents {
			if c.GetType() == "file" && c.GetName() == "pubspec.yaml" {
				return "", nil
			}
		}
		return "", fmt.Errorf("pubspec.yaml not found in %s/%s", owner, repo)
	}
	return dir, nil
}
