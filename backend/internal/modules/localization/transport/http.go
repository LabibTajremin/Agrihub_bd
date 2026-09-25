// Package transport exposes dictionaries and voice manifests over HTTP.
package transport

import (
	"fmt"
	"net/http"
	"time"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/localization/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/localization/usecase"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authz"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/httpx"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/validator"
)

// Language is one supported language.
type Language struct {
	Code       string `json:"code"`
	Name       string `json:"name"`
	NativeName string `json:"native_name"`
	IsRTL      bool   `json:"is_rtl"`
	IsDefault  bool   `json:"is_default"`
	HasVoice   bool   `json:"has_voice"`
	Version    int64  `json:"version"`
}

// Languages lists languages.
type Languages struct {
	Languages []Language `json:"languages"`
}

// SinceQuery selects a delta.
type SinceQuery struct {
	Since int64 `query:"since"`
}

// Dictionary is a snapshot (full=true) or delta.
type Dictionary struct {
	Lang    string            `json:"lang"`
	Version int64             `json:"version"`
	Full    bool              `json:"full"`
	Entries map[string]string `json:"entries"`
}

// Clip is one voice asset in a manifest.
type Clip struct {
	URL        string `json:"url"`
	DurationMs int    `json:"duration_ms"`
	Checksum   string `json:"checksum"`
}

// VoiceManifest maps keys to clips.
type VoiceManifest struct {
	Lang    string          `json:"lang"`
	Version int64           `json:"version"`
	Full    bool            `json:"full"`
	Clips   map[string]Clip `json:"clips"`
}

// UpsertEntriesRequest edits a dictionary.
type UpsertEntriesRequest struct {
	Entries map[string]string `json:"entries" validate:"required,min=1,max=5000"`
}

// UpsertResponse reports the new version.
type UpsertResponse struct {
	Version int64 `json:"version"`
	Changed int   `json:"changed"`
}

// VoiceAssetInput registers one clip.
type VoiceAssetInput struct {
	Key        string `json:"key" validate:"required,i18nkey"`
	MediaID    string `json:"media_id" validate:"omitempty,uuid"`
	URL        string `json:"url" validate:"required,url"`
	DurationMs int    `json:"duration_ms" validate:"gt=0,lte=600000"`
	Checksum   string `json:"checksum" validate:"required,len=64,hexadecimal"`
}

// UpsertVoiceRequest registers clips.
type UpsertVoiceRequest struct {
	Assets []VoiceAssetInput `json:"assets" validate:"required,min=1,max=2000,dive"`
}

// Handlers groups the localization endpoints.
type Handlers struct {
	List        usecase.ListLanguages
	Get         usecase.GetDictionary
	Voice       usecase.GetVoiceManifest
	Upsert      usecase.UpsertEntries
	UpsertVoice usecase.UpsertVoice
	V           *validator.Validator
	MaxAge      time.Duration
}

// Routes declares the endpoints.
func (h Handlers) Routes() []httpx.Route {
	return []httpx.Route{
		{Method: http.MethodGet, Path: "/v1/i18n/languages", Tag: "i18n", Summary: "Supported languages", Response: Languages{}, Handler: h.languages},
		{Method: http.MethodGet, Path: "/v1/i18n/{lang}", Tag: "i18n", Summary: "Dictionary snapshot or delta (ETag/304)", Query: SinceQuery{}, Response: Dictionary{}, Handler: h.dictionary},
		{Method: http.MethodPut, Path: "/v1/i18n/{lang}/entries", Permission: string(authz.DictionaryWrite), Tag: "i18n", Summary: "Upsert dictionary entries",
			Request: UpsertEntriesRequest{}, Response: UpsertResponse{}, Handler: h.upsert},
		{Method: http.MethodGet, Path: "/v1/voice/{lang}", Tag: "i18n", Summary: "Voice clip manifest (ETag/304)", Query: SinceQuery{}, Response: VoiceManifest{}, Handler: h.voice},
		{Method: http.MethodPut, Path: "/v1/voice/{lang}/assets", Permission: string(authz.VoiceWrite), Tag: "i18n", Summary: "Register pre-recorded voice clips",
			Request: UpsertVoiceRequest{}, Response: UpsertResponse{}, Handler: h.upsertVoice},
	}
}

func (h Handlers) languages(w http.ResponseWriter, r *http.Request) error {
	ls, err := h.List.Execute(r.Context())
	if err != nil {
		return err
	}
	out := Languages{Languages: make([]Language, len(ls))}
	for i, l := range ls {
		out.Languages[i] = Language{Code: l.Code, Name: l.Name, NativeName: l.NativeName, IsRTL: l.IsRTL, IsDefault: l.IsDefault, HasVoice: l.HasVoice, Version: l.Version}
	}
	return httpx.JSON(w, http.StatusOK, out)
}

func since(r *http.Request) (*int64, error) {
	if r.URL.Query().Get("since") == "" {
		return nil, nil
	}
	v, err := httpx.QueryInt(r, "since", 0, 0, 1<<62)
	return &v, err
}

// cached writes caching headers and reports whether the client copy is current.
func (h Handlers) cached(w http.ResponseWriter, r *http.Request, etag string) bool {
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", int(h.MaxAge.Seconds())))
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return true
	}
	return false
}

func (h Handlers) dictionary(w http.ResponseWriter, r *http.Request) error {
	s, err := since(r)
	if err != nil {
		return err
	}
	d, err := h.Get.Execute(r.Context(), r.PathValue("lang"), s)
	if err != nil {
		return err
	}
	if h.cached(w, r, d.ETag) {
		return nil
	}
	return httpx.JSON(w, http.StatusOK, Dictionary{Lang: d.Lang, Version: d.Version, Full: d.Full, Entries: d.Entries})
}

func (h Handlers) voice(w http.ResponseWriter, r *http.Request) error {
	s, err := since(r)
	if err != nil {
		return err
	}
	m, err := h.Voice.Execute(r.Context(), r.PathValue("lang"), s)
	if err != nil {
		return err
	}
	if h.cached(w, r, m.ETag) {
		return nil
	}
	out := VoiceManifest{Lang: m.Lang, Version: m.Version, Full: m.Full, Clips: make(map[string]Clip, len(m.Clips))}
	for _, c := range m.Clips {
		out.Clips[c.Key] = Clip{URL: c.URL, DurationMs: c.DurationMs, Checksum: c.Checksum}
	}
	return httpx.JSON(w, http.StatusOK, out)
}

func (h Handlers) upsert(w http.ResponseWriter, r *http.Request) error {
	var in UpsertEntriesRequest
	if err := httpx.Decode(r, &in, h.V); err != nil {
		return err
	}
	res, err := h.Upsert.Execute(r.Context(), r.PathValue("lang"), in.Entries)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, UpsertResponse{Version: res.Version, Changed: res.Changed})
}

func (h Handlers) upsertVoice(w http.ResponseWriter, r *http.Request) error {
	var in UpsertVoiceRequest
	if err := httpx.Decode(r, &in, h.V); err != nil {
		return err
	}
	assets := make([]domain.VoiceAsset, len(in.Assets))
	for i, a := range in.Assets {
		assets[i] = domain.VoiceAsset{Key: a.Key, MediaID: a.MediaID, URL: a.URL, DurationMs: a.DurationMs, Checksum: a.Checksum}
	}
	v, err := h.UpsertVoice.Execute(r.Context(), r.PathValue("lang"), assets)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, UpsertResponse{Version: v, Changed: len(assets)})
}
