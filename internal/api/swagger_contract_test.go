// Package api — Phase 1 OpenAPI contract tests (TDD red suite).
//
// This file derives the failing test suite for the Phase 1 contract
// (specs/api-contract.md) purely from the generated Swagger 2.0 document at
// core-api/docs/api/swagger.json. It needs no external infrastructure
// (no Postgres, Redis, network, Keycloak or GitHub).
//
// The swagger.json currently committed is stale (empty info block, free-form
// response schemas, missing routes such as /healthz and /envs, stale
// /internal/kubernetes/* and /github/pubspec-path paths, no operationIds,
// missing per-operation security), so most subtests FAIL (red). They must turn
// GREEN once the Developer completes Phase 1 (annotation fixes, typed models,
// error-envelope standardization, spec regeneration into core-api/docs/api/).
//
// Every assertion is made against the JSON document itself; the canonical
// route matrix and stale-path list are embedded below from Table 2-1 and §2.3
// of the contract.
package api

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	re2xx       = regexp.MustCompile(`^2\d\d$`)
	reSemver    = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	rePathParam = regexp.MustCompile(`\{([A-Za-z0-9_]+)\}`)
)

// httpMethods is the set of Swagger 2.0 operation keys (lowercase) we scan.
var httpMethods = []string{"get", "post", "put", "delete", "patch", "head", "options"}

// route mirrors one row of Table 2-1 of the contract.
type route struct {
	method string // lowercase HTTP method as keyed under "paths"
	path   string
	public bool // true = public (O, no auth), false = protected (P)
}

// expectedRoutes is the canonical 50-operation route matrix (contract Table 2-1).
var expectedRoutes = []route{
	{"post", "/auth/register", true},
	{"post", "/auth/login", true},
	{"post", "/auth/refresh", true},
	{"post", "/auth/logout", true},
	{"get", "/healthz", true},
	{"get", "/flutter/versions", false},
	{"get", "/auth/@me", false},
	{"put", "/auth/@me", false},
	{"get", "/env", false},
	{"post", "/env", false},
	{"get", "/envs", false},
	{"get", "/env/{envId}", false},
	{"put", "/env/{envId}", false},
	{"delete", "/env/{envId}", false},
	{"get", "/project", false},
	{"post", "/project", false},
	{"get", "/project/{id}", false},
	{"put", "/project/{id}", false},
	{"delete", "/project/{id}", false},
	{"get", "/project/{id}/config", false},
	{"post", "/project/{id}/config", false},
	{"delete", "/project/{id}/config", false},
	{"get", "/keystore", false},
	{"post", "/keystore", false},
	{"get", "/keystores", false},
	{"delete", "/keystore/{keystoreId}", false},
	{"get", "/google-play-credentials", false},
	{"post", "/google-play-credentials", false},
	{"delete", "/google-play-credentials/{credentialsId}", false},
	{"post", "/project/{id}/build", false},
	{"delete", "/project/{id}/build/{buildId}", false},
	{"put", "/project/{id}/build/{buildId}/cancel", false},
	{"get", "/project/{id}/builds", false},
	{"get", "/project/{id}/build/{buildId}/logs", false},
	{"get", "/project/{id}/build/{buildId}/download", false},
	{"get", "/project/{id}/build/{buildId}/logs/sync", false},
	{"delete", "/project/{id}/cache", false},
	{"get", "/project/{id}/cache/metrics", false},
	{"get", "/project/{id}/cache/entries", false},
	{"post", "/project/{id}/build/{buildId}/publish", false},
	{"get", "/project/{id}/release/{releaseId}", false},
	{"get", "/project/{id}/releases", false},
	{"get", "/project/{id}/google-play/access", false},
	{"get", "/project/{id}/audit", false},
	{"post", "/github/post-installation", false},
	{"get", "/github/repos", false},
	{"get", "/github/repo", false},
	{"get", "/github/installations", false},
	{"get", "/github/installation", false},
	{"delete", "/github/disconnect", false},
}

// stalePaths is the §2.3 list that MUST never appear in the spec.
var stalePaths = []string{
	"/internal/kubernetes/pod",
	"/internal/kubernetes/pod/{buildID}",
	"/internal/kubernetes/pod/{buildID}/logs",
	"/internal/kubernetes/pod/{buildID}/logs/stream",
	"/internal/kubernetes/pod/{buildID}/status",
	"/internal/kubernetes/pod/{buildID}/listen",
	"/internal/kubernetes/pod/{buildID}/artifact/{artifactName}",
	"/internal/kubernetes/pod/{buildID}/artifacts",
	"/internal/kubernetes/configmap",
	"/internal/kubernetes/secret",
	"/internal/kubernetes/build/{buildID}/resources",
	"/github/pubspec-path",
}

// contractPathVars are the only template variables the contract documents as
// integer path parameters (Table §6.1).
var contractPathVars = []string{
	"{id}", "{envId}", "{keystoreId}", "{credentialsId}", "{buildId}", "{releaseId}",
}

// ---------------------------------------------------------------------------
// Generic JSON helpers (spec is parsed with encoding/json into generic maps).
// ---------------------------------------------------------------------------

func obj(v interface{}) map[string]interface{} {
	m, _ := v.(map[string]interface{})
	return m
}

func arr(v interface{}) []interface{} {
	s, _ := v.([]interface{})
	return s
}

func str(v interface{}) string {
	s, _ := v.(string)
	return s
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Spec loading.
// ---------------------------------------------------------------------------

// findSwaggerJSON locates core-api/docs/api/swagger.json by walking up from the
// test working directory (go test runs from the package dir), so the suite is
// robust to the repo being checked out anywhere.
func findSwaggerJSON(t *testing.T) string {
	t.Helper()
	start, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	dir := start
	for {
		candidate := filepath.Join(dir, "docs", "api", "swagger.json")
		if fi, err := os.Stat(candidate); err == nil && !fi.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate docs/api/swagger.json walking up from %s", start)
		}
		dir = parent
	}
}

func loadSwaggerSpec(t *testing.T) map[string]interface{} {
	t.Helper()
	path := findSwaggerJSON(t)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var spec map[string]interface{}
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("parsing %s as JSON: %v", path, err)
	}
	return spec
}

func specPaths(spec map[string]interface{}) map[string]interface{} {
	return obj(spec["paths"])
}

// opEntry is one documented operation: (method, path) → operation object.
type opEntry struct {
	method string
	path   string
	op     map[string]interface{}
}

// specOps enumerates every (method, path) operation in the spec.
func specOps(spec map[string]interface{}) []opEntry {
	var out []opEntry
	for p, pv := range specPaths(spec) {
		item := obj(pv)
		if item == nil {
			continue
		}
		for _, m := range httpMethods {
			ov, ok := item[m]
			if !ok {
				continue
			}
			out = append(out, opEntry{method: m, path: p, op: obj(ov)})
		}
	}
	return out
}

// expectedAuth looks up the public/protected flag of a route in Table 2-1.
func expectedAuth(method, path string) (public bool, found bool) {
	for _, r := range expectedRoutes {
		if r.method == method && r.path == path {
			return r.public, true
		}
	}
	return false, false
}

// ---------------------------------------------------------------------------
// TestSwagger — the whole suite, one subtest per acceptance criterion.
// ---------------------------------------------------------------------------

func TestSwagger(t *testing.T) {
	spec := loadSwaggerSpec(t)
	ops := specOps(spec)

	// Guard the embedded route matrix itself against accidental corruption:
	// the contract is 50 operations over 39 unique paths, each row unique.
	if n := len(expectedRoutes); n != 50 {
		t.Fatalf("test table corruption: expectedRoutes has %d rows, contract Table 2-1 has 50", n)
	}
	uniqPaths := map[string]bool{}
	uniqOps := map[string]bool{}
	for _, r := range expectedRoutes {
		uniqPaths[r.path] = true
		key := r.method + " " + r.path
		if uniqOps[key] {
			t.Fatalf("test table corruption: duplicate route %s", key)
		}
		uniqOps[key] = true
	}
	if n := len(uniqPaths); n != 39 {
		t.Fatalf("test table corruption: expectedRoutes span %d unique paths, contract says 39", n)
	}

	t.Run("AC-1 route coverage", func(t *testing.T) { testAC1(t, spec, ops) })
	t.Run("AC-2 stale paths", func(t *testing.T) { testAC2(t, spec) })
	t.Run("AC-3 bare paths", func(t *testing.T) { testAC3(t, spec, ops) })
	t.Run("AC-4 title", func(t *testing.T) { testAC4(t, spec) })
	t.Run("AC-5 version", func(t *testing.T) { testAC5(t, spec) })
	t.Run("AC-6 metadata completeness", func(t *testing.T) { testAC6(t, spec) })
	t.Run("AC-7 security scheme", func(t *testing.T) { testAC7(t, spec) })
	t.Run("AC-8 per-operation security", func(t *testing.T) { testAC8(t, ops) })
	t.Run("AC-9 typed failures", func(t *testing.T) { testAC9(t, spec, ops) })
	t.Run("AC-11 typed successes", func(t *testing.T) { testAC11(t, spec, ops) })
	t.Run("AC-13 path parameters", func(t *testing.T) { testAC13(t, ops) })
	t.Run("AC-15 operation completeness", func(t *testing.T) { testAC15(t, ops) })
	t.Run("AC-16 envelope definitions", func(t *testing.T) { testAC16(t, spec) })
	t.Run("AC-17 no dangling refs", func(t *testing.T) { testAC17(t, spec, ops) })
	t.Run("AC-21 health endpoint", func(t *testing.T) { testAC21(t, ops) })
	t.Run("AC-22 post-installation method contract", func(t *testing.T) { testAC22(t, ops) })
}

// testAC1 — AC-1: the (method, path) set in the spec equals Table 2-1 exactly
// (39 unique paths / 50 operations), with no missing and no extra entries.
func testAC1(t *testing.T, spec map[string]interface{}, ops []opEntry) {
	if v := str(spec["swagger"]); v != "2.0" {
		t.Errorf("AC-1: spec must declare swagger \"2.0\", got %q", v)
	}

	present := map[string]bool{}
	for _, e := range ops {
		present[e.method+" "+e.path] = true
	}

	for _, r := range expectedRoutes {
		key := r.method + " " + r.path
		if !present[key] {
			t.Errorf("AC-1: missing operation %s %s (must be documented per Table 2-1)",
				strings.ToUpper(r.method), r.path)
		}
	}

	for _, e := range ops {
		if _, found := expectedAuth(e.method, e.path); !found {
			t.Errorf("AC-1: unexpected operation %s %s (not in Table 2-1)",
				strings.ToUpper(e.method), e.path)
		}
	}

	if got := len(specPaths(spec)); got != 39 {
		t.Errorf("AC-1: expected exactly 39 unique paths, got %d", got)
	}
	if got := len(ops); got != 50 {
		t.Errorf("AC-1: expected exactly 50 operations, got %d", got)
	}
}

// testAC2 — AC-2: none of the 12 stale paths of §2.3 appear in the spec.
func testAC2(t *testing.T, spec map[string]interface{}) {
	paths := specPaths(spec)
	for _, p := range stalePaths {
		if _, ok := paths[p]; ok {
			t.Errorf("AC-2: stale path %s must NOT appear in the spec (§2.3)", p)
		}
	}
}

// testAC3 — AC-3: bare paths only; no /api/v1 prefix, no host/basePath fields.
func testAC3(t *testing.T, spec map[string]interface{}, ops []opEntry) {
	for _, e := range ops {
		if strings.HasPrefix(e.path, "/api/v1") {
			t.Errorf("AC-3: path %s must not carry an /api/v1 prefix (D1)", e.path)
		}
	}
	if v, ok := spec["host"]; ok && str(v) != "" {
		t.Errorf("AC-3: spec must not set host, got %q", str(v))
	}
	if v, ok := spec["basePath"]; ok && str(v) != "" {
		t.Errorf("AC-3: spec must not set basePath, got %q", str(v))
	}
}

// testAC4 — AC-4: info.title is exactly "Flotio Core API".
func testAC4(t *testing.T, spec map[string]interface{}) {
	info := obj(spec["info"])
	if got := str(info["title"]); got != "Flotio Core API" {
		t.Errorf("AC-4: info.title must be exactly %q, got %q", "Flotio Core API", got)
	}
}

// testAC5 — AC-5: info.version is a semver string ^\d+\.\d+\.\d+$.
func testAC5(t *testing.T, spec map[string]interface{}) {
	info := obj(spec["info"])
	ver := str(info["version"])
	if !reSemver.MatchString(ver) {
		t.Errorf("AC-5: info.version must match ^\\d+\\.\\d+\\.\\d+$, got %q", ver)
	}
}

// testAC6 — AC-6: description, contact.name, contact.email, license.name non-empty.
func testAC6(t *testing.T, spec map[string]interface{}) {
	info := obj(spec["info"])
	if strings.TrimSpace(str(info["description"])) == "" {
		t.Errorf("AC-6: info.description must be non-empty")
	}
	contact := obj(info["contact"])
	if strings.TrimSpace(str(contact["name"])) == "" {
		t.Errorf("AC-6: info.contact.name must be non-empty")
	}
	if strings.TrimSpace(str(contact["email"])) == "" {
		t.Errorf("AC-6: info.contact.email must be non-empty")
	}
	license := obj(info["license"])
	if strings.TrimSpace(str(license["name"])) == "" {
		t.Errorf("AC-6: info.license.name must be non-empty")
	}
}

// testAC7 — AC-7: securityDefinitions declares exactly BearerAuth as an apiKey
// in header named Authorization, with a non-empty description (M9).
func testAC7(t *testing.T, spec map[string]interface{}) {
	sd := obj(spec["securityDefinitions"])
	ba := obj(sd["BearerAuth"])
	if got := str(ba["type"]); got != "apiKey" {
		t.Errorf("AC-7: BearerAuth.type must be \"apiKey\", got %q", got)
	}
	if got := str(ba["in"]); got != "header" {
		t.Errorf("AC-7: BearerAuth.in must be \"header\", got %q", got)
	}
	if got := str(ba["name"]); got != "Authorization" {
		t.Errorf("AC-7: BearerAuth.name must be \"Authorization\", got %q", got)
	}
	if strings.TrimSpace(str(ba["description"])) == "" {
		t.Errorf("AC-7: BearerAuth.description must be non-empty")
	}
	for k := range sd {
		if k != "BearerAuth" {
			t.Errorf("AC-7: unexpected security scheme %q (M9: exactly BearerAuth)", k)
		}
	}
}

// testAC8 — AC-8: every protected operation declares security [{BearerAuth: []}];
// every public operation declares none.
func testAC8(t *testing.T, ops []opEntry) {
	for _, e := range ops {
		public, inTable := expectedAuth(e.method, e.path)
		if !inTable {
			continue // stale/extra ops are AC-1's concern
		}
		label := strings.ToUpper(e.method) + " " + e.path
		sec := arr(e.op["security"])
		if public {
			if len(sec) > 0 {
				t.Errorf("AC-8: public operation %s must not declare security", label)
			}
			continue
		}
		if len(sec) == 0 {
			t.Errorf("AC-8: protected operation %s must declare security [{BearerAuth: []}]", label)
			continue
		}
		declaresBearer := false
		for _, req := range sec {
			if m := obj(req); m != nil {
				if _, ok := m["BearerAuth"]; ok {
					declaresBearer = true
					break
				}
			}
		}
		if !declaresBearer {
			t.Errorf("AC-8: protected operation %s must reference BearerAuth in its security requirements", label)
		}
	}
}

// testAC9 — AC-9: every failure (non-2xx) response schema is a $ref to
// APIErrorResponse (ref path ends with "APIErrorResponse") and resolves.
func testAC9(t *testing.T, spec map[string]interface{}, ops []opEntry) {
	defs := obj(spec["definitions"])
	for _, e := range ops {
		label := strings.ToUpper(e.method) + " " + e.path
		for code, rv := range obj(e.op["responses"]) {
			if re2xx.MatchString(code) {
				continue
			}
			schema := obj(obj(rv)["schema"])
			ref := str(schema["$ref"])
			if !strings.HasSuffix(ref, "APIErrorResponse") {
				t.Errorf("AC-9: %s failure response %s must $ref APIErrorResponse, got %q", label, code, ref)
				continue
			}
			defName := strings.TrimPrefix(ref, "#/definitions/")
			if _, ok := defs[defName]; !ok {
				t.Errorf("AC-9: %s failure response %s $ref %q is dangling", label, code, ref)
			}
		}
	}
}

// isFreeFormObject reports whether a schema is an inline free-form object
// (map[string]interface{} / map[string]string style) rather than a $ref or a
// typed primitive/array schema.
func isFreeFormObject(s map[string]interface{}) bool {
	if s == nil {
		return false
	}
	if _, hasRef := s["$ref"]; hasRef {
		return false
	}
	if _, hasAdditional := s["additionalProperties"]; hasAdditional {
		return true
	}
	return str(s["type"]) == "object"
}

// testAC11 — AC-11: zero success response schemas are inline free-form objects;
// every success schema either $refs a named definition or is a typed (non-free
// form) schema; http-only statuses (204/205/304) may omit a schema.
func testAC11(t *testing.T, spec map[string]interface{}, ops []opEntry) {
	defs := obj(spec["definitions"])
	for _, e := range ops {
		label := strings.ToUpper(e.method) + " " + e.path
		for code, rv := range obj(e.op["responses"]) {
			if !re2xx.MatchString(code) {
				continue
			}
			rm := obj(rv)
			if rm == nil {
				continue // unparseable response object; AC-17/valid-spec territory
			}
			schema := obj(rm["schema"])
			if schema == nil {
				if code == "204" || code == "205" || code == "304" {
					continue // http-only status: no schema required
				}
				t.Errorf("AC-11: %s success response %s has no schema (must $ref a named definition)", label, code)
				continue
			}
			if ref := str(schema["$ref"]); ref != "" {
				defName := strings.TrimPrefix(ref, "#/definitions/")
				if _, ok := defs[defName]; !ok {
					t.Errorf("AC-11: %s success response %s $ref %q is dangling", label, code, ref)
				}
				continue
			}
			if isFreeFormObject(schema) {
				t.Errorf("AC-11: %s success response %s is an inline free-form object schema (must $ref a named definition)", label, code)
			}
		}
	}
}

// testAC13 — AC-13: every operation on a path containing one of the contract's
// integer template variables declares that variable as an in:path parameter of
// type integer with required:true (§6.1).
func testAC13(t *testing.T, ops []opEntry) {
	for _, e := range ops {
		hasContractVar := false
		for _, v := range contractPathVars {
			if strings.Contains(e.path, v) {
				hasContractVar = true
				break
			}
		}
		if !hasContractVar {
			continue
		}
		label := strings.ToUpper(e.method) + " " + e.path
		params := arr(e.op["parameters"])
		for _, m := range rePathParam.FindAllStringSubmatch(e.path, -1) {
			name := m[1]
			var match map[string]interface{}
			for _, pv := range params {
				if pm := obj(pv); pm != nil && str(pm["name"]) == name {
					match = pm
					break
				}
			}
			if match == nil {
				t.Errorf("AC-13: %s must declare an in:path parameter %q", label, name)
				continue
			}
			if got := str(match["in"]); got != "path" {
				t.Errorf("AC-13: %s parameter %q must be in:path, got %q", label, name, got)
			}
			if req, ok := match["required"]; !ok || req != true {
				t.Errorf("AC-13: %s parameter %q must be required:true", label, name)
			}
			if got := str(match["type"]); got != "integer" {
				t.Errorf("AC-13: %s parameter %q must be type integer, got %q", label, name, got)
			}
		}
	}
}

// testAC15 — AC-15: every operation has a non-empty summary, a unique non-empty
// operationId, at least one tag, produces containing application/json, and at
// least one 2xx success response.
func testAC15(t *testing.T, ops []opEntry) {
	seenOpIDs := map[string]string{}
	for _, e := range ops {
		label := strings.ToUpper(e.method) + " " + e.path

		if strings.TrimSpace(str(e.op["summary"])) == "" {
			t.Errorf("AC-15: %s must have a non-empty summary", label)
		}

		oid := str(e.op["operationId"])
		if strings.TrimSpace(oid) == "" {
			t.Errorf("AC-15: %s must have a non-empty operationId", label)
		} else if prev, dup := seenOpIDs[oid]; dup {
			t.Errorf("AC-15: operationId %q is not unique (%s and %s)", oid, prev, label)
		} else {
			seenOpIDs[oid] = label
		}

		if len(arr(e.op["tags"])) == 0 {
			t.Errorf("AC-15: %s must declare at least one tag", label)
		}

		hasJSON := false
		for _, p := range arr(e.op["produces"]) {
			if str(p) == "application/json" {
				hasJSON = true
			}
		}
		if !hasJSON {
			t.Errorf("AC-15: %s must declare produces application/json", label)
		}

		has2xx := false
		for code := range obj(e.op["responses"]) {
			if re2xx.MatchString(code) {
				has2xx = true
			}
		}
		if !has2xx {
			t.Errorf("AC-15: %s must document at least one 2xx success response", label)
		}
	}
}

// testAC16 — AC-16: definitions contains APIErrorResponse with exactly
// {status, code, message} and APIResponse with exactly {status, code, message,
// details} (§4.1), with the documented property types.
func testAC16(t *testing.T, spec map[string]interface{}) {
	defs := obj(spec["definitions"])

	checkEnvelope := func(name string, wantProps map[string]string) {
		d := obj(defs[name])
		if d == nil {
			var candidates []string
			for k := range defs {
				if strings.HasSuffix(k, name) {
					candidates = append(candidates, k)
				}
			}
			if len(candidates) == 0 {
				t.Errorf("AC-16: definitions must contain %q (contract §4.1); no similar definition found", name)
			} else {
				t.Errorf("AC-16: definitions must contain a definition named exactly %q (contract §4.1); closest matches: %s",
					name, strings.Join(candidates, ", "))
			}
			return
		}
		props := obj(d["properties"])
		for _, want := range []string{"status", "code", "message", "details"} {
			ps := obj(props[want])
			wantType, mustBeTyped := wantProps[want]
			if want == "details" {
				// details may be untyped ({}) per §4.1
				if _, present := props["details"]; !present {
					t.Errorf("AC-16: %s must have property \"details\"", name)
				}
				continue
			}
			if ps == nil {
				t.Errorf("AC-16: %s must have property %q", name, want)
				continue
			}
			if mustBeTyped {
				if got := str(ps["type"]); got != wantType {
					t.Errorf("AC-16: %s property %q must have type %q, got %q", name, want, wantType, got)
				}
			}
		}
		for k := range props {
			if !contains([]string{"status", "code", "message", "details"}, k) {
				t.Errorf("AC-16: %s must not have extra property %q", name, k)
			}
		}
	}

	checkEnvelope("APIErrorResponse", map[string]string{"status": "string", "code": "integer", "message": "string"})
	checkEnvelope("APIResponse", map[string]string{"status": "string", "code": "integer", "message": "string"})
}

// testAC17 — AC-17: no dangling $refs; every $ref in response schemas and body
// parameters resolves to an existing definition.
func testAC17(t *testing.T, spec map[string]interface{}, ops []opEntry) {
	defs := obj(spec["definitions"])
	check := func(where, ref string) {
		if ref == "" {
			return
		}
		if !strings.HasPrefix(ref, "#/definitions/") {
			t.Errorf("AC-17: %s has non-definition $ref %q", where, ref)
			return
		}
		if _, ok := defs[strings.TrimPrefix(ref, "#/definitions/")]; !ok {
			t.Errorf("AC-17: dangling $ref %q at %s", ref, where)
		}
	}
	for _, e := range ops {
		label := strings.ToUpper(e.method) + " " + e.path
		for code, rv := range obj(e.op["responses"]) {
			schema := obj(obj(rv)["schema"])
			check(fmt.Sprintf("%s response %s", label, code), str(schema["$ref"]))
		}
		for _, pv := range arr(e.op["parameters"]) {
			p := obj(pv)
			if p == nil || str(p["in"]) != "body" {
				continue
			}
			schema := obj(p["schema"])
			check(fmt.Sprintf("%s body parameter %q", label, str(p["name"])), str(schema["$ref"]))
		}
	}
}

// testAC21 — AC-21: GET /healthz is documented with a 200 response and remains
// public (no security).
func testAC21(t *testing.T, ops []opEntry) {
	var op map[string]interface{}
	for _, e := range ops {
		if e.method == "get" && e.path == "/healthz" {
			op = e.op
		}
	}
	if op == nil {
		t.Errorf("AC-21: GET /healthz must be documented")
		return
	}
	if sec := arr(op["security"]); len(sec) > 0 {
		t.Errorf("AC-21: GET /healthz must remain public (no security)")
	}
	if _, ok := obj(op["responses"])["200"]; !ok {
		t.Errorf("AC-21: GET /healthz must document a 200 response")
	}
}

// testAC22 — AC-22: POST /github/post-installation is documented and carries a
// 405 in its failure set (handler rejects non-POST with 405).
func testAC22(t *testing.T, ops []opEntry) {
	var op map[string]interface{}
	for _, e := range ops {
		if e.method == "post" && e.path == "/github/post-installation" {
			op = e.op
		}
	}
	if op == nil {
		t.Errorf("AC-22: POST /github/post-installation must be documented")
		return
	}
	if _, ok := obj(op["responses"])["405"]; !ok {
		t.Errorf("AC-22: POST /github/post-installation must document a 405 response")
	}
}
