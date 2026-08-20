package service

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"

	dbEngine "github.com/flotio-dev/core-api/internal/common/database"
	githubEngine "github.com/flotio-dev/core-api/internal/infra/github"
	githubModels "github.com/flotio-dev/core-api/internal/modules/github/model"
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

func (s *GithubService) SaveInstallation(userID uint, installationID int64, accountLogin, accountType string, targetID int64, avatarURL string) error {
	return s.Repository.SaveInstallation(userID, installationID, accountLogin, accountType, targetID, avatarURL)
}

func (s *GithubService) GetInstallationByUser(userID uint) (*dbEngine.GithubInstallation, error) {
	return s.Repository.GetInstallationByUser(userID)
}

func (s *GithubService) ListInstallationsByUser(userID uint) ([]dbEngine.GithubInstallation, error) {
	return s.Repository.ListInstallationsByUser(userID)
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
		resp, derr := client.Apps.DeleteInstallation(ctx, installationID)
		if derr != nil && (resp == nil || resp.StatusCode != 404) {
			return derr
		}
	} else {
		return err
	}

	// suppression en base
	if err := s.Repository.DeleteInstallationByInstallationID(installationID); err != nil {
		return err
	}

	return nil
}

// DeleteUserInstallation supprime le lien de l'utilisateur avec l'installation en base.
// Si aucun autre utilisateur ne partage cette installation, elle est également supprimée sur GitHub.
func (s *GithubService) DeleteUserInstallation(ctx context.Context, userID uint, installationID int64) error {
	if err := s.Repository.DeleteInstallationByUser(userID); err != nil {
		return err
	}

	otherCount, err := s.Repository.CountOtherInstallations(installationID, userID)
	if err != nil {
		return nil
	}

	if otherCount > 0 {
		return nil
	}

	client, err := s.ClientManager.ClientForApp()
	if err == nil {
		resp, derr := client.Apps.DeleteInstallation(ctx, installationID)
		if derr != nil && (resp == nil || resp.StatusCode != 404) {
			return derr
		}
	}

	return nil
}

// DeleteUserInstallationByID supprime le lien de l'utilisateur avec une installation précise.
// Si aucun autre utilisateur ne partage cette installation, elle est également supprimée sur GitHub.
func (s *GithubService) DeleteUserInstallationByID(ctx context.Context, userID uint, installationID int64) error {
	if err := s.Repository.DeleteUserInstallationByID(userID, installationID); err != nil {
		return err
	}

	otherCount, err := s.Repository.CountOtherInstallations(installationID, userID)
	if err != nil {
		return nil
	}

	if otherCount > 0 {
		return nil
	}

	client, err := s.ClientManager.ClientForApp()
	if err == nil {
		resp, derr := client.Apps.DeleteInstallation(ctx, installationID)
		if derr != nil && (resp == nil || resp.StatusCode != 404) {
			return derr
		}
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

func (s *GithubService) DetectFlutterProject(ctx context.Context, installationID int64, owner, repo string) (*githubModels.FlutterProjectDetection, error) {
	client, err := s.ClientManager.ClientForInstallation(installationID)
	if err != nil {
		return nil, fmt.Errorf("cannot create github client: %w", err)
	}

	result := &githubModels.FlutterProjectDetection{
		ProjectPath: ".",
	}

	// Helper to fetch file content
	getFile := func(filePath string) (string, error) {
		fileContent, _, resp, err := client.Repositories.GetContents(ctx, owner, repo, filePath, nil)
		if err != nil {
			return "", err
		}
		if resp != nil && resp.StatusCode == 404 {
			return "", fmt.Errorf("not found")
		}
		if fileContent == nil {
			return "", fmt.Errorf("not a file")
		}
		content, err := fileContent.GetContent()
		if err != nil {
			return "", err
		}
		return content, nil
	}

	// 1. Check FVM config at root (.fvm/fvm_config.json)
	if fvmContent, err := getFile(".fvm/fvm_config.json"); err == nil {
		var fvm struct {
			Flutter           string `json:"flutter"`
			FlutterSdkVersion string `json:"flutterSdkVersion"`
		}
		if json.Unmarshal([]byte(fvmContent), &fvm) == nil {
			v := fvm.Flutter
			if v == "" {
				v = fvm.FlutterSdkVersion
			}
			if v != "" {
				result.DetectedFlutterVersion = v
				result.DetectionSource = "fvm"
			}
		}
	}

	// 2. Check .flutter-version at root
	if result.DetectedFlutterVersion == "" {
		if verContent, err := getFile(".flutter-version"); err == nil {
			v := strings.TrimSpace(verContent)
			if v != "" {
				result.DetectedFlutterVersion = v
				result.DetectionSource = "flutter-version"
			}
		}
	}

	// 3. Find project path (where pubspec.yaml is located)
	buildPath, _ := s.FindBuildPath(ctx, installationID, owner, repo)
	if buildPath != "" {
		result.ProjectPath = buildPath
	} else {
		result.ProjectPath = "."
	}

	// 4. If version still not found and project_path != ., check FVM or .flutter-version in project path
	if result.DetectedFlutterVersion == "" && result.ProjectPath != "." && result.ProjectPath != "" {
		if fvmContent, err := getFile(path.Join(result.ProjectPath, ".fvm/fvm_config.json")); err == nil {
			var fvm struct {
				Flutter           string `json:"flutter"`
				FlutterSdkVersion string `json:"flutterSdkVersion"`
			}
			if json.Unmarshal([]byte(fvmContent), &fvm) == nil {
				v := fvm.Flutter
				if v == "" {
					v = fvm.FlutterSdkVersion
				}
				if v != "" {
					result.DetectedFlutterVersion = v
					result.DetectionSource = "fvm"
				}
			}
		}

		if result.DetectedFlutterVersion == "" {
			if verContent, err := getFile(path.Join(result.ProjectPath, ".flutter-version")); err == nil {
				v := strings.TrimSpace(verContent)
				if v != "" {
					result.DetectedFlutterVersion = v
					result.DetectionSource = "flutter-version"
				}
			}
		}
	}

	// 5. If version still not found, parse pubspec.yaml
	if result.DetectedFlutterVersion == "" {
		pubspecPath := "pubspec.yaml"
		if result.ProjectPath != "." && result.ProjectPath != "" {
			pubspecPath = path.Join(result.ProjectPath, "pubspec.yaml")
		}
		if pubspecContent, err := getFile(pubspecPath); err == nil {
			reFlutter := regexp.MustCompile(`(?m)^\s*flutter:\s*["']?([><=^\s]*([0-9]+\.[0-9]+\.[0-9]+))["']?`)
			if matches := reFlutter.FindStringSubmatch(pubspecContent); len(matches) >= 3 {
				result.DetectedFlutterVersion = matches[2]
				result.DetectionSource = "pubspec"
			}
		}
	}

	// 6. Check for google-services.json
	gsPath := "android/app/google-services.json"
	if result.ProjectPath != "." && result.ProjectPath != "" {
		gsPath = path.Join(result.ProjectPath, gsPath)
	}
	if _, err := getFile(gsPath); err == nil {
		result.HasGoogleServices = true
	}

	return result, nil
}

func (s *GithubService) GetGithubInstallationByUserID(userID uint) (*dbEngine.GithubInstallation, error) {
	return s.Repository.GetInstallationByUser(userID)
}

func (s *GithubService) UpdateInstallation(userID uint, installationID int64) error {
	return s.Repository.SaveInstallation(userID, installationID, "", "", 0, "")
}
