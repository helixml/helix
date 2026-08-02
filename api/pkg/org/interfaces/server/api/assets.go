package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	assetapp "github.com/helixml/helix/api/pkg/org/application/assets"
	"github.com/helixml/helix/api/pkg/org/domain/asset"
	"github.com/helixml/helix/api/pkg/org/domain/store"
)

type ServerAssetDTO struct {
	Address            string         `json:"address"`
	Port               uint16         `json:"port"`
	User               string         `json:"user"`
	AuthType           asset.AuthType `json:"auth_type"`
	PublicKey          string         `json:"public_key,omitempty"`
	PasswordConfigured bool           `json:"password_configured"`
	HostKeyFingerprint string         `json:"host_key_fingerprint,omitempty"`
}

type AssetDTO struct {
	ID             string          `json:"id"`
	OrganizationID string          `json:"organization_id"`
	Name           string          `json:"name"`
	Description    string          `json:"description,omitempty"`
	NotesForAgents string          `json:"notes_for_agents,omitempty"`
	Enabled        bool            `json:"enabled"`
	Kind           asset.Kind      `json:"kind"`
	Server         *ServerAssetDTO `json:"server,omitempty"`
	AgentIDs       []string        `json:"agent_ids"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

type AssetsResponse struct {
	Assets []AssetDTO `json:"assets"`
}

type ServerAssetWriteRequest struct {
	Address  string         `json:"address"`
	Port     uint16         `json:"port,omitempty"`
	User     string         `json:"user"`
	AuthType asset.AuthType `json:"auth_type"`
	Password string         `json:"password,omitempty"`
	HostKey  string         `json:"host_key,omitempty"`
}

type CreateAssetRequest struct {
	Name           string                   `json:"name"`
	Description    string                   `json:"description,omitempty"`
	NotesForAgents string                   `json:"notes_for_agents,omitempty"`
	Kind           asset.Kind               `json:"kind"`
	Server         *ServerAssetWriteRequest `json:"server,omitempty"`
}

type UpdateServerAssetRequest struct {
	Address  *string         `json:"address,omitempty"`
	Port     *uint16         `json:"port,omitempty"`
	User     *string         `json:"user,omitempty"`
	AuthType *asset.AuthType `json:"auth_type,omitempty"`
	Password *string         `json:"password,omitempty"`
	HostKey  *string         `json:"host_key,omitempty"`
}

type UpdateAssetRequest struct {
	Name           *string                   `json:"name,omitempty"`
	Description    *string                   `json:"description,omitempty"`
	NotesForAgents *string                   `json:"notes_for_agents,omitempty"`
	Enabled        *bool                     `json:"enabled,omitempty"`
	Server         *UpdateServerAssetRequest `json:"server,omitempty"`
}

type AssetLinkRequest struct {
	AgentID string `json:"agent_id"`
}

type AssetLinksResponse struct {
	AgentIDs []string `json:"agent_ids"`
}

type AssetHealthDTO struct {
	TCPReachable bool      `json:"tcp_reachable"`
	SSHReachable bool      `json:"ssh_reachable"`
	LatencyMS    int64     `json:"latency_ms"`
	Error        string    `json:"error,omitempty"`
	CheckedAt    time.Time `json:"checked_at"`
}

func (a *apiHandler) requireAssets(w http.ResponseWriter) bool {
	if a.deps.Assets == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("assets service not configured"))
		return false
	}
	return true
}

func assetErrorStatus(err error) int {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, store.ErrConflict):
		return http.StatusConflict
	default:
		return http.StatusBadRequest
	}
}

func (a *apiHandler) assetDTO(ctxOrg string, value asset.Asset, agentIDs []string) AssetDTO {
	dto := AssetDTO{
		ID: value.ID, OrganizationID: ctxOrg, Name: value.Name,
		Description: value.Description, NotesForAgents: value.NotesForAgents,
		Enabled: !value.Disabled, Kind: value.Kind, AgentIDs: agentIDs, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
	if value.Config.Server != nil {
		s := value.Config.Server
		dto.Server = &ServerAssetDTO{
			Address: s.Address, Port: s.Port, User: s.User, AuthType: s.AuthType,
			PublicKey: s.PublicKey, PasswordConfigured: s.EncryptedPassword != "",
			HostKeyFingerprint: hostKeyFingerprint(s.HostKey),
		}
	}
	return dto
}

func (a *apiHandler) dtoWithLinks(r *http.Request, orgID string, value asset.Asset) (AssetDTO, error) {
	links, err := a.deps.Assets.ListLinks(r.Context(), orgID, value.ID)
	if err != nil {
		return AssetDTO{}, err
	}
	agentIDs := make([]string, 0, len(links))
	for _, link := range links {
		agentIDs = append(agentIDs, link.AgentID)
	}
	return a.assetDTO(orgID, value, agentIDs), nil
}

// @Summary Helix-org: list assets
// @Tags HelixOrg
// @Produce json
// @Success 200 {object} api.AssetsResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/assets [get]
func (a *apiHandler) listAssets(w http.ResponseWriter, r *http.Request) {
	if !a.requireAssets(w) {
		return
	}
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	values, err := a.deps.Assets.List(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("list assets: %w", err))
		return
	}
	out := make([]AssetDTO, 0, len(values))
	for _, value := range values {
		dto, err := a.dtoWithLinks(r, orgID, value)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("list asset links: %w", err))
			return
		}
		out = append(out, dto)
	}
	writeJSON(w, http.StatusOK, AssetsResponse{Assets: out})
}

// @Summary Helix-org: create an asset
// @Tags HelixOrg
// @Accept json
// @Produce json
// @Param payload body api.CreateAssetRequest true "Asset spec"
// @Success 201 {object} api.AssetDTO
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/assets [post]
func (a *apiHandler) createAsset(w http.ResponseWriter, r *http.Request) {
	if !a.requireAssets(w) {
		return
	}
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var req CreateAssetRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Kind != asset.KindServer || req.Server == nil {
		writeError(w, http.StatusBadRequest, errors.New("server asset config is required"))
		return
	}
	value, err := a.deps.Assets.CreateServer(r.Context(), orgID, assetapp.CreateServerParams{
		Name: req.Name, Description: req.Description, NotesForAgents: req.NotesForAgents,
		Address: req.Server.Address, Port: req.Server.Port, User: req.Server.User,
		AuthType: req.Server.AuthType, Password: req.Server.Password, HostKey: req.Server.HostKey,
	})
	if err != nil {
		writeError(w, assetErrorStatus(err), err)
		return
	}
	writeJSON(w, http.StatusCreated, a.assetDTO(orgID, value, []string{}))
}

// @Summary Helix-org: get an asset
// @Tags HelixOrg
// @Produce json
// @Success 200 {object} api.AssetDTO
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/assets/{id} [get]
func (a *apiHandler) getAsset(w http.ResponseWriter, r *http.Request) {
	if !a.requireAssets(w) {
		return
	}
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	value, err := a.deps.Assets.Get(r.Context(), orgID, r.PathValue("id"))
	if err != nil {
		writeError(w, assetErrorStatus(err), err)
		return
	}
	dto, err := a.dtoWithLinks(r, orgID, value)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, dto)
}

// @Summary Helix-org: update an asset
// @Tags HelixOrg
// @Accept json
// @Produce json
// @Param payload body api.UpdateAssetRequest true "Asset patch"
// @Success 200 {object} api.AssetDTO
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/assets/{id} [patch]
func (a *apiHandler) updateAsset(w http.ResponseWriter, r *http.Request) {
	if !a.requireAssets(w) {
		return
	}
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var req UpdateAssetRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	params := assetapp.UpdateServerParams{Name: req.Name, Description: req.Description, NotesForAgents: req.NotesForAgents, Enabled: req.Enabled}
	if req.Server != nil {
		params.Address = req.Server.Address
		params.Port = req.Server.Port
		params.User = req.Server.User
		params.AuthType = req.Server.AuthType
		params.Password = req.Server.Password
		params.HostKey = req.Server.HostKey
	}
	value, err := a.deps.Assets.UpdateServer(r.Context(), orgID, r.PathValue("id"), params)
	if err != nil {
		writeError(w, assetErrorStatus(err), err)
		return
	}
	dto, err := a.dtoWithLinks(r, orgID, value)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, dto)
}

// @Summary Helix-org: delete an asset
// @Tags HelixOrg
// @Success 204 "No Content"
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/assets/{id} [delete]
func (a *apiHandler) deleteAsset(w http.ResponseWriter, r *http.Request) {
	if !a.requireAssets(w) {
		return
	}
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := a.deps.Assets.Delete(r.Context(), orgID, r.PathValue("id")); err != nil {
		writeError(w, assetErrorStatus(err), err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// @Summary Helix-org: check asset health
// @Tags HelixOrg
// @Produce json
// @Success 200 {object} api.AssetHealthDTO
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/assets/{id}/health [get]
func (a *apiHandler) assetHealth(w http.ResponseWriter, r *http.Request) {
	if !a.requireAssets(w) {
		return
	}
	if a.deps.AssetHealth == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("asset health checker not configured"))
		return
	}
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, a.deps.AssetHealth(r.Context(), orgID, r.PathValue("id")))
}

// @Summary Helix-org: list asset links
// @Tags HelixOrg
// @Produce json
// @Success 200 {object} api.AssetLinksResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/assets/{id}/links [get]
func (a *apiHandler) listAssetLinks(w http.ResponseWriter, r *http.Request) {
	if !a.requireAssets(w) {
		return
	}
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	links, err := a.deps.Assets.ListLinks(r.Context(), orgID, r.PathValue("id"))
	if err != nil {
		writeError(w, assetErrorStatus(err), err)
		return
	}
	agentIDs := make([]string, 0, len(links))
	for _, link := range links {
		agentIDs = append(agentIDs, link.AgentID)
	}
	writeJSON(w, http.StatusOK, AssetLinksResponse{AgentIDs: agentIDs})
}

// @Summary Helix-org: link an asset to an agent
// @Tags HelixOrg
// @Accept json
// @Produce json
// @Param payload body api.AssetLinkRequest true "Agent link"
// @Success 201 {object} asset.Link
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/assets/{id}/links [post]
func (a *apiHandler) linkAsset(w http.ResponseWriter, r *http.Request) {
	if !a.requireAssets(w) {
		return
	}
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var req AssetLinkRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.AgentID) == "" {
		writeError(w, http.StatusBadRequest, errors.New("agent_id is required"))
		return
	}
	link, err := a.deps.Assets.Link(r.Context(), orgID, r.PathValue("id"), req.AgentID)
	if err != nil {
		writeError(w, assetErrorStatus(err), err)
		return
	}
	writeJSON(w, http.StatusCreated, link)
}

// @Summary Helix-org: unlink an asset from an agent
// @Tags HelixOrg
// @Success 204 "No Content"
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/assets/{id}/links/{agent_id} [delete]
func (a *apiHandler) unlinkAsset(w http.ResponseWriter, r *http.Request) {
	if !a.requireAssets(w) {
		return
	}
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := a.deps.Assets.Unlink(r.Context(), orgID, r.PathValue("id"), r.PathValue("agent_id")); err != nil {
		writeError(w, assetErrorStatus(err), err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func hostKeyFingerprint(hostKey string) string {
	if hostKey == "" {
		return ""
	}
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(hostKey))
	if err != nil {
		return ""
	}
	return ssh.FingerprintSHA256(key)
}
