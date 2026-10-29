package main

// Tests that keep docs/api true to the code: the OpenAPI spec's routes,
// schemas and examples, and the signing test vectors.
//
// Regenerate the vectors (keeping their keys) with:
//
//	go test -run TestVectors -update-vectors .

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"delsignbackend/chain"
	"delsignbackend/users"
	"delsignbackend/wallets"
	"encoding/hex"
	"encoding/json"
	"flag"
	"math/big"
	"os"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	specPath    = "docs/api/openapi.yaml"
	vectorsPath = "docs/api/test-vectors.json"
)

var updateVectors = flag.Bool("update-vectors", false, "regenerate "+vectorsPath)

// The parts of an OpenAPI document the tests look at.
type openAPI struct {
	Paths      map[string]map[string]operation `yaml:"paths"`
	Components struct {
		Schemas map[string]schema `yaml:"schemas"`
	} `yaml:"components"`
}

type operation struct {
	OperationID string `yaml:"operationId"`
	RequestBody *struct {
		Content map[string]mediaType `yaml:"content"`
	} `yaml:"requestBody"`
	Responses map[string]struct {
		Content map[string]mediaType `yaml:"content"`
	} `yaml:"responses"`
}

type mediaType struct {
	Schema   schema `yaml:"schema"`
	Example  any    `yaml:"example"`
	Examples map[string]struct {
		Value any `yaml:"value"`
	} `yaml:"examples"`
}

type schema struct {
	Ref        string         `yaml:"$ref"`
	Type       string         `yaml:"type"`
	Properties map[string]any `yaml:"properties"`
	Required   []string       `yaml:"required"`
	Items      *schema        `yaml:"items"`
	Extra      map[string]any `yaml:",inline"` // every other keyword
}

// schemaTypes maps each object schema in the spec to the Go type the server
// decodes or encodes it with.
var schemaTypes = map[string]reflect.Type{
	"KeyReg":            reflect.TypeFor[users.KeyReg](),
	"UserInfo":          reflect.TypeFor[users.UserInfo](),
	"WalletInfo":        reflect.TypeFor[wallets.WalletInfo](),
	"EOA":               reflect.TypeFor[wallets.EOA](),
	"WalletAddressPair": reflect.TypeFor[wallets.WalletAddressPair](),
	"Balance":           reflect.TypeFor[chain.Balance](),
	"SendRequest":       reflect.TypeFor[chain.SendPayload](),
	"SendResult":        reflect.TypeFor[chain.SendResult](),
}

func loadSpec(t *testing.T) *openAPI {
	t.Helper()
	data, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	var spec openAPI
	if err := yaml.Unmarshal(data, &spec); err != nil {
		t.Fatalf("parse %s: %v", specPath, err)
	}
	return &spec
}

func TestOpenAPIRefsResolve(t *testing.T) {
	data, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}

	var walk func(node any)
	walk = func(node any) {
		switch n := node.(type) {
		case map[string]any:
			if ref, ok := n["$ref"].(string); ok {
				// Follow "#/a/b/c" from the document root
				var target any = doc
				for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
					m, _ := target.(map[string]any)
					target = m[part]
				}
				if target == nil {
					t.Errorf("$ref %s doesn't resolve", ref)
				}
			}
			for _, v := range n {
				walk(v)
			}
		case []any:
			for _, v := range n {
				walk(v)
			}
		}
	}
	walk(doc)
}

func TestOpenAPIMatchesRoutes(t *testing.T) {
	spec := loadSpec(t)

	var inSpec, inServer []string
	for path, ops := range spec.Paths {
		for method := range ops {
			inSpec = append(inSpec, strings.ToUpper(method)+" "+path)
		}
	}
	for _, route := range apiRoutes {
		inServer = append(inServer, route.pattern)
	}
	sort.Strings(inSpec)
	sort.Strings(inServer)
	if !slices.Equal(inSpec, inServer) {
		t.Errorf("routes differ\nspec:   %q\nserver: %q", inSpec, inServer)
	}
}

// jsonFields returns the JSON names of typ's fields, and which are required
// (not omitempty).
func jsonFields(typ reflect.Type) (all, required []string) {
	for f := range typ.Fields() {
		name, opts, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "" || name == "-" {
			continue
		}
		all = append(all, name)
		if !strings.Contains(opts, "omitempty") {
			required = append(required, name)
		}
	}
	sort.Strings(all)
	sort.Strings(required)
	return all, required
}

func TestOpenAPISchemasMatchGoTypes(t *testing.T) {
	spec := loadSpec(t)

	for name, s := range spec.Components.Schemas {
		if s.Type != "object" {
			continue
		}
		typ, ok := schemaTypes[name]
		if !ok {
			t.Errorf("schema %s has no Go type in schemaTypes", name)
			continue
		}
		var props []string
		for p := range s.Properties {
			props = append(props, p)
		}
		sort.Strings(props)
		required := slices.Sorted(slices.Values(s.Required))

		wantAll, wantRequired := jsonFields(typ)
		if !slices.Equal(props, wantAll) {
			t.Errorf("schema %s properties %q, %s has %q", name, props, typ, wantAll)
		}
		if !slices.Equal(required, wantRequired) {
			t.Errorf("schema %s required %q, %s requires %q", name, required, typ, wantRequired)
		}
		if s.Extra["additionalProperties"] != false {
			t.Errorf("schema %s should set additionalProperties: false", name)
		}
	}
	for name := range schemaTypes {
		if _, ok := spec.Components.Schemas[name]; !ok {
			t.Errorf("schemaTypes has %s, the spec doesn't", name)
		}
	}
}

// goType returns the Go type for a schema: a $ref to an object schema, or an
// array of one, or of integers.
func goType(t *testing.T, s schema) reflect.Type {
	t.Helper()
	switch {
	case s.Ref != "":
		typ, ok := schemaTypes[strings.TrimPrefix(s.Ref, "#/components/schemas/")]
		if !ok {
			t.Fatalf("no Go type for %s", s.Ref)
		}
		return typ
	case s.Type == "array" && s.Items != nil && s.Items.Type == "integer":
		return reflect.TypeFor[[]int]()
	case s.Type == "array" && s.Items != nil:
		return reflect.SliceOf(goType(t, *s.Items))
	}
	t.Fatalf("no Go type for schema %+v", s)
	return nil
}

// decodeStrict decodes the example into a new value of typ the way the server
// decodes request bodies: unknown fields are an error.
func decodeStrict(example any, typ reflect.Type) error {
	data, err := json.Marshal(example)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return dec.Decode(reflect.New(typ).Interface())
}

func TestOpenAPIExamplesDecode(t *testing.T) {
	spec := loadSpec(t)

	for path, ops := range spec.Paths {
		for method, op := range ops {
			where := strings.ToUpper(method) + " " + path
			if op.RequestBody != nil {
				mt := op.RequestBody.Content["application/json"]
				if len(mt.Examples) == 0 {
					t.Errorf("%s: request body has no examples", where)
				}
				for name, ex := range mt.Examples {
					if err := decodeStrict(ex.Value, goType(t, mt.Schema)); err != nil {
						t.Errorf("%s: request example %s: %v", where, name, err)
					}
				}
			}
			for status, resp := range op.Responses {
				mt, ok := resp.Content["application/json"]
				if !ok {
					continue
				}
				if mt.Example == nil {
					t.Errorf("%s %s: no example", where, status)
					continue
				}
				if err := decodeStrict(mt.Example, goType(t, mt.Schema)); err != nil {
					t.Errorf("%s %s: example: %v", where, status, err)
				}
			}
		}
	}
}

// The spec's request examples are the test vectors' bodies, so they're real,
// verifiable requests.
func TestOpenAPIExamplesAreVectors(t *testing.T) {
	spec := loadSpec(t)
	vf := readVectors(t)
	body := func(name string) any {
		for _, v := range vf.Vectors {
			if v.Name == name {
				var b any
				if err := json.Unmarshal(v.Body, &b); err != nil {
					t.Fatal(err)
				}
				return b
			}
		}
		t.Fatalf("no vector %q", name)
		return nil
	}
	example := func(path, example string) any {
		ex := spec.Paths[path]["post"].RequestBody.Content["application/json"].Examples[example].Value
		// Round-trip through JSON so numbers compare the same way
		data, _ := json.Marshal(ex)
		var v any
		json.Unmarshal(data, &v)
		return v
	}

	checks := []struct{ path, example, vector string }{
		{"/api/v1/keyreg", "firstRegistration", "register"},
		{"/api/v1/keyreg", "rotation", "rotate"},
		{"/api/v1/wallets/send", "send", "send"},
	}
	for _, c := range checks {
		if got, want := example(c.path, c.example), body(c.vector); !reflect.DeepEqual(got, want) {
			t.Errorf("%s example %s differs from vector %s\nspec:   %v\nvector: %v", c.path, c.example, c.vector, got, want)
		}
	}
}

// Signing test vectors (docs/api/test-vectors.json).

type vectorKey struct {
	PublicKeySpkiHex   string `json:"publicKeySpkiHex"`
	PrivateKeyPkcs8Hex string `json:"privateKeyPkcs8Hex"`
}

type vectorSignature struct {
	Field     string `json:"field"`
	Key       string `json:"key"`
	Message   string `json:"message"`
	Signature string `json:"signature"`
}

type vector struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Request     string            `json:"request"`
	Signatures  []vectorSignature `json:"signatures"`
	Body        json.RawMessage   `json:"body"`
}

type vectorFile struct {
	Description string               `json:"description"`
	Algorithm   string               `json:"algorithm"`
	Keys        map[string]vectorKey `json:"keys"`
	Vectors     []vector             `json:"vectors"`
}

func readVectors(t *testing.T) *vectorFile {
	t.Helper()
	data, err := os.ReadFile(vectorsPath)
	if err != nil {
		t.Fatal(err)
	}
	var vf vectorFile
	if err := json.Unmarshal(data, &vf); err != nil {
		t.Fatalf("parse %s: %v", vectorsPath, err)
	}
	return &vf
}

func newVectorKey(t *testing.T) vectorKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	priv, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	return vectorKey{PublicKeySpkiHex: hex.EncodeToString(pub), PrivateKeyPkcs8Hex: hex.EncodeToString(priv)}
}

// signWith signs msg with a vector key. PKCS #1 v1.5 is deterministic, so the
// same key and message always give the same signature.
func signWith(t *testing.T, k vectorKey, msg string) string {
	t.Helper()
	der, err := hex.DecodeString(k.PrivateKeyPkcs8Hex)
	if err != nil {
		t.Fatal(err)
	}
	key, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(msg))
	sig, err := rsa.SignPKCS1v15(nil, key.(*rsa.PrivateKey), crypto.SHA256, hash[:])
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(sig)
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// buildVectors makes the vector file from two keys, using the server's own
// message builders.
func buildVectors(t *testing.T, keys map[string]vectorKey) *vectorFile {
	t.Helper()
	const email = "a@example.com"
	k1, k2 := keys["key1"], keys["key2"]

	sigReg := signWith(t, k1, email)
	register := vector{
		Name:        "register",
		Description: "First registration of key1. sig4reg is key1's signature over the email.",
		Request:     "POST /api/v1/keyreg",
		Signatures:  []vectorSignature{{"sig4reg", "key1", email, sigReg}},
		Body:        mustJSON(t, users.KeyReg{Email: email, PubKey: k1.PublicKeySpkiHex, SignatureForRegistration: sigReg}),
	}

	rotMsg := users.RotationMessage(email, k2.PublicKeySpkiHex)
	sigReg2, sigRot := signWith(t, k2, email), signWith(t, k1, rotMsg)
	rotate := vector{
		Name: "rotate",
		Description: "Replacing key1 with key2. sig4reg is the new key's (key2) signature over the email; " +
			"sig4rotate is the current key's (key1) signature over the email followed by key2's public key hex.",
		Request: "POST /api/v1/keyreg",
		Signatures: []vectorSignature{
			{"sig4reg", "key2", email, sigReg2},
			{"sig4rotate", "key1", rotMsg, sigRot},
		},
		Body: mustJSON(t, users.KeyReg{Email: email, PubKey: k2.PublicKeySpkiHex, SignatureForRegistration: sigReg2, SignatureForRotation: sigRot}),
	}

	send := func(name, description, source, dest, amount string) vector {
		wei, _ := new(big.Int).SetString(amount, 10)
		p := chain.SendPayload{
			SourceAddress:      source,
			DestinationAddress: dest,
			Amount:             chain.Wei{Int: wei},
			RequestId:          "6f1c2a7e-0b9d-4c55-9a43-2f1e8d7c6b5a",
			Expiry:             1800000060,
		}
		msg := chain.SendMessage(&p)
		p.Signature = signWith(t, k1, msg)
		return vector{
			Name:        name,
			Description: description,
			Request:     "POST /api/v1/wallets/send",
			Signatures:  []vectorSignature{{"sig", "key1", msg, p.Signature}},
			Body:        mustJSON(t, p),
		}
	}
	const source, dest = "0x17155B71982feD535Fd6aeB084C9330e7d5C290f", "0x812B8C4D02f5901C7Dcdac0526bA915B61dAc33c"

	return &vectorFile{
		Description: "Signing test vectors for the delsign API. The keys are test keys, published here; " +
			"never use them for anything else. Each vector gives a request body and, for each signature in it, " +
			"the exact message signed. A client's signing code should reproduce every signature exactly. " +
			"Send vectors are signed with key1 and have a fixed expiry, so a live server rejects them " +
			"(expiry must be within the next 5 minutes); they're for testing signing code. Generated by apidocs_test.go; don't edit by hand.",
		Algorithm: "RSASSA-PKCS1-v1_5 with SHA-256 over the UTF-8 message. Keys and signatures are hex: " +
			"public keys are SPKI (PKIX) DER, private keys PKCS #8 DER.",
		Keys: keys,
		Vectors: []vector{
			register,
			rotate,
			send("send", "Send 0.001 ETH. The message is source + dest + amount + requestId + expiry.",
				source, dest, "1000000000000000"),
			send("send, lowercase addresses", "The same send with the addresses in lowercase. The server accepts "+
				"either case, but the signature covers the addresses exactly as sent, so the signature differs.",
				strings.ToLower(source), strings.ToLower(dest), "1000000000000000"),
			send("send, large amount", "An amount above 2^53, which a JavaScript number can't hold exactly. "+
				"Keep amounts as strings or BigInt; the message has the decimal digits as sent.",
				source, dest, "123456789012345678901234567890"),
		},
	}
}

func marshalVectors(t *testing.T, vf *vectorFile) []byte {
	t.Helper()
	data, err := json.MarshalIndent(vf, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(data, '\n')
}

func TestVectors(t *testing.T) {
	if *updateVectors {
		keys := map[string]vectorKey{}
		if data, err := os.ReadFile(vectorsPath); err == nil {
			var old vectorFile
			if err := json.Unmarshal(data, &old); err != nil {
				t.Fatal(err)
			}
			keys = old.Keys
		}
		for _, name := range []string{"key1", "key2"} {
			if _, ok := keys[name]; !ok {
				keys[name] = newVectorKey(t)
			}
		}
		if err := os.WriteFile(vectorsPath, marshalVectors(t, buildVectors(t, keys)), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	data, err := os.ReadFile(vectorsPath)
	if err != nil {
		t.Fatal(err)
	}
	vf := readVectors(t)

	// The file is exactly what the generator makes from its keys, so a hand
	// edit or a change to a message format fails here
	if want := marshalVectors(t, buildVectors(t, vf.Keys)); !bytes.Equal(data, want) {
		t.Fatalf("%s is out of date; regenerate it with: go test -run TestVectors -update-vectors .", vectorsPath)
	}

	// Each signature is in the body, verifies with the server's verification
	// code, and signs the message the server rebuilds from the body
	for _, v := range vf.Vectors {
		var fields map[string]any
		if err := json.Unmarshal(v.Body, &fields); err != nil {
			t.Fatal(err)
		}
		for _, s := range v.Signatures {
			if fields[s.Field] != s.Signature {
				t.Errorf("%s: body %s isn't the listed signature", v.Name, s.Field)
			}
			if s.Field == "sig4reg" && fields["pubkey"] != vf.Keys[s.Key].PublicKeySpkiHex {
				t.Errorf("%s: sig4reg is by %s, but pubkey isn't %s's", v.Name, s.Key, s.Key)
			}
			if err := users.VerifySignature(vf.Keys[s.Key].PublicKeySpkiHex, s.Message, s.Signature); err != nil {
				t.Errorf("%s: %s doesn't verify: %v", v.Name, s.Field, err)
			}
			var serverMsg string
			switch v.Request {
			case "POST /api/v1/keyreg":
				var kr users.KeyReg
				if err := decodeStrict(v.Body, reflect.TypeFor[users.KeyReg]()); err != nil {
					t.Fatalf("%s: body: %v", v.Name, err)
				}
				json.Unmarshal(v.Body, &kr)
				serverMsg = kr.Email
				if s.Field == "sig4rotate" {
					serverMsg = users.RotationMessage(kr.Email, kr.PubKey)
				}
			case "POST /api/v1/wallets/send":
				var p chain.SendPayload
				if err := decodeStrict(v.Body, reflect.TypeFor[chain.SendPayload]()); err != nil {
					t.Fatalf("%s: body: %v", v.Name, err)
				}
				json.Unmarshal(v.Body, &p)
				serverMsg = chain.SendMessage(&p)
			}
			if serverMsg != s.Message {
				t.Errorf("%s: %s message %q, server builds %q", v.Name, s.Field, s.Message, serverMsg)
			}
		}
	}
}
