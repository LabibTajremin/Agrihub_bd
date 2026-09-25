// Package authz holds the static RBAC permission matrix and ownership policy.
// Use cases call these checks; transport checks are only a fast-fail.
package authz

import (
	"sort"
	"strings"

	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/errs"
)

// Role is a principal's role.
type Role string

// Roles, from least to most privileged.
const (
	Guest        Role = "guest"
	Farmer       Role = "farmer"
	FieldOfficer Role = "field_officer"
	Agronomist   Role = "agronomist"
	Admin        Role = "admin"
)

// Permission is "resource:action".
type Permission string

// Permissions.
const (
	ProfileRead      Permission = "profile:read"
	ProfileWrite     Permission = "profile:write"
	ScanCreate       Permission = "scan:create"
	ScanRead         Permission = "scan:read"
	ScanReadAny      Permission = "scan:read_any"
	ScanSync         Permission = "scan:sync"
	FieldCreate      Permission = "field:create"
	FieldRead        Permission = "field:read"
	FieldReadAny     Permission = "field:read_any"
	FieldWrite       Permission = "field:write"
	FieldWriteAny    Permission = "field:write_any"
	CropRead         Permission = "crop:read"
	AdvisoryRead     Permission = "advisory:read"
	WeatherRead      Permission = "weather:read"
	AlertRead        Permission = "alert:read"
	AlertWrite       Permission = "alert:write"
	SubscriptionEdit Permission = "subscription:write"
	DictionaryRead   Permission = "dictionary:read"
	DictionaryWrite  Permission = "dictionary:write"
	VoiceWrite       Permission = "voice:write"
	MediaUpload      Permission = "media:upload"
	MediaRead        Permission = "media:read"
	AssistantAsk     Permission = "assistant:ask"
	UserManage       Permission = "user:manage"
	MetricsRead      Permission = "metrics:read"
)

// Roles lists every role.
func Roles() []Role { return []Role{Guest, Farmer, FieldOfficer, Agronomist, Admin} }

// Permissions lists every permission, sorted.
func Permissions() []Permission {
	out := []Permission{ProfileRead, ProfileWrite, ScanCreate, ScanRead, ScanReadAny, ScanSync,
		FieldCreate, FieldRead, FieldReadAny, FieldWrite, FieldWriteAny, CropRead, AdvisoryRead,
		WeatherRead, AlertRead, AlertWrite, SubscriptionEdit, DictionaryRead, DictionaryWrite,
		VoiceWrite, MediaUpload, MediaRead, AssistantAsk, UserManage, MetricsRead}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

var (
	guestPerms = []Permission{ProfileRead, ScanCreate, ScanRead, MediaUpload, MediaRead, CropRead,
		AdvisoryRead, WeatherRead, DictionaryRead, AssistantAsk}
	farmerPerms  = append(append([]Permission(nil), guestPerms...), ProfileWrite, ScanSync, FieldCreate, FieldRead, FieldWrite, AlertRead, SubscriptionEdit)
	officerPerms = append(append([]Permission(nil), farmerPerms...), FieldReadAny, ScanReadAny)
	agroPerms    = append(append([]Permission(nil), officerPerms...), AlertWrite)
)

// matrix is immutable after package initialisation (a constant table).
var matrix = map[Role]map[Permission]bool{
	Guest:        set(guestPerms),
	Farmer:       set(farmerPerms),
	FieldOfficer: set(officerPerms),
	Agronomist:   set(agroPerms),
	Admin:        set(Permissions()),
}

func set(ps []Permission) map[Permission]bool {
	m := make(map[Permission]bool, len(ps))
	for _, p := range ps {
		m[p] = true
	}
	return m
}

// ValidRole reports whether r is a known role.
func ValidRole(r Role) bool {
	_, ok := matrix[r]
	return ok
}

// Can reports whether role holds permission.
func Can(role Role, p Permission) bool { return matrix[role][p] }

// PermissionsOf lists the permissions of role, sorted.
func PermissionsOf(role Role) []Permission {
	out := make([]Permission, 0, len(matrix[role]))
	for p := range matrix[role] {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// ErrForbidden is returned when a permission or ownership check fails.
var ErrForbidden = errs.Forbidden("auth.forbidden")

// Check returns ErrForbidden unless role holds p.
func Check(role Role, p Permission) error {
	if !Can(role, p) {
		return ErrForbidden.WithField("permission", string(p))
	}
	return nil
}

// Owned is the ownership policy: a principal may act on a resource when it
// holds the "any" permission, or holds the "own" permission and owns it.
func Owned(role Role, subject, ownerID string, own, anyPerm Permission) error {
	if Can(role, anyPerm) || (Can(role, own) && subject != "" && subject == ownerID) {
		return nil
	}
	return ErrForbidden.WithField("permission", string(own))
}

// Render prints the matrix in a stable text form (golden-tested).
func Render() string {
	var b strings.Builder
	for _, r := range Roles() {
		b.WriteString(string(r))
		b.WriteString(":\n")
		for _, p := range PermissionsOf(r) {
			b.WriteString("  " + string(p) + "\n")
		}
	}
	return b.String()
}
