package handler

import (
	"encoding/json"
	"net/http"
	"time"

	helpers "github.com/flotio-dev/core-api/internal/common/server"
)

type FlutterController struct{}

func NewFlutterController() *FlutterController {
	return &FlutterController{}
}

type FlutterVersion struct {
	Version string `json:"version"`
	Channel string `json:"channel"`
}

type FlutterVersionsResponse struct {
	Versions []FlutterVersion `json:"versions"`
}

var versionsCache []FlutterVersion
var lastFetch time.Time

// VersionsGetHandler godoc
//
//	@Summary		Get available Flutter versions
//	@Description	Get a list of available Flutter versions from the official releases
//	@Tags			flutter
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	FlutterVersionsResponse
//	@Router			/flutter/versions [get]
func (c *FlutterController) VersionsGetHandler(w http.ResponseWriter, r *http.Request) {
	if time.Since(lastFetch) < 1*time.Hour && versionsCache != nil {
		helpers.WriteJSON(w, FlutterVersionsResponse{Versions: versionsCache})
		return
	}

	// Fetch from Google Storage
	resp, err := http.Get("https://storage.googleapis.com/flutter_infra_release/releases/releases_linux.json")
	if err != nil {
		// Fallback to some common versions if fetch fails
		fallback := []FlutterVersion{
			{Version: "3.22.0", Channel: "stable"},
			{Version: "3.19.0", Channel: "stable"},
			{Version: "3.16.0", Channel: "stable"},
		}
		helpers.WriteJSON(w, FlutterVersionsResponse{Versions: fallback})
		return
	}
	defer resp.Body.Close()

	var data struct {
		Releases []struct {
			Version string `json:"version"`
			Channel string `json:"channel"`
		} `json:"releases"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		helpers.WriteErrorJSON(w, "Failed to parse Flutter versions", http.StatusInternalServerError)
		return
	}

	versions := make([]FlutterVersion, 0)
	seen := make(map[string]bool)

	for _, rel := range data.Releases {
		if rel.Channel == "stable" && !seen[rel.Version] {
			versions = append(versions, FlutterVersion{
				Version: rel.Version,
				Channel: rel.Channel,
			})
			seen[rel.Version] = true
		}
	}

	versionsCache = versions
	lastFetch = time.Now()

	helpers.WriteJSON(w, FlutterVersionsResponse{Versions: versions})
}
