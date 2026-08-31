package plugin

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newSignatureTestKey(t *testing.T) (privatePEM, publicPEM string) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate ECDSA key: %v", err)
	}
	privDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal EC private key: %v", err)
	}
	privatePEM = string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privDER}))

	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	publicPEM = string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}))
	return privatePEM, publicPEM
}

func TestSignVerifyRoundtrip(t *testing.T) {
	privPEM, pubPEM := newSignatureTestKey(t)
	data := []byte(`{"name":"demo","version":"1.0.0","url":"https://example.com/demo.zip"}`)

	sig, err := Sign(data, privPEM)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if sig == "" {
		t.Fatal("expected non-empty signature")
	}
	if err := Verify(data, sig, pubPEM); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func TestVerifyRejectsTamperedData(t *testing.T) {
	privPEM, pubPEM := newSignatureTestKey(t)
	data := []byte("original data")
	sig, err := Sign(data, privPEM)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	tampered := []byte("tampered data")
	if err := Verify(tampered, sig, pubPEM); err == nil {
		t.Fatal("expected Verify to fail for tampered data")
	}
}

func TestTrustStoreRejectsUnknownPublicKey(t *testing.T) {
	privPEM, pubPEM := newSignatureTestKey(t)

	ts := NewTrustStore()
	if err := ts.AddPublicKey("key-1", pubPEM); err != nil {
		t.Fatalf("AddPublicKey: %v", err)
	}

	data := []byte("trust me")
	sig, err := Sign(data, privPEM)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	err = ts.VerifyWithTrust(data, Signature{PublicKeyID: "not-in-store", Signature: sig})
	if err == nil {
		t.Fatal("expected unknown public key error")
	}
	if !errors.Is(err, ErrUnknownPublicKey) {
		t.Fatalf("expected ErrUnknownPublicKey, got %v", err)
	}
}

func TestInstallFromMarketWithSignaturePasses(t *testing.T) {
	privPEM, pubPEM := newSignatureTestKey(t)
	payload, sum := makePluginZip(t, "signed", "1.0.0")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	entry := MarketIndex{
		Name:    "signed",
		Version: "1.0.0",
		URL:     srv.URL + "/signed.zip",
		SHA256:  sum,
	}
	manifestData, err := manifestDataForSignature(entry)
	if err != nil {
		t.Fatalf("manifestDataForSignature: %v", err)
	}
	sig, err := Sign(manifestData, privPEM)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	entry.Signature = Signature{PublicKeyID: "market-key", Signature: sig}

	ts := NewTrustStore()
	if err := ts.AddPublicKey("market-key", pubPEM); err != nil {
		t.Fatalf("AddPublicKey: %v", err)
	}

	dest := t.TempDir()
	p, err := InstallFromMarket(context.Background(), []MarketIndex{entry}, "signed", dest, ts)
	if err != nil {
		t.Fatalf("InstallFromMarket with valid signature: %v", err)
	}
	if p.Manifest.Name != "signed" {
		t.Fatalf("unexpected installed plugin: %#v", p.Manifest)
	}
	if _, err := os.Stat(filepath.Join(dest, "signed", "signed"+ManifestSuffix)); err != nil {
		t.Fatalf("installed manifest not found: %v", err)
	}
}

func TestInstallFromMarketWithSignatureRejectsTamperedEntry(t *testing.T) {
	privPEM, pubPEM := newSignatureTestKey(t)
	payload, sum := makePluginZip(t, "tampered", "1.0.0")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	entry := MarketIndex{
		Name:    "tampered",
		Version: "1.0.0",
		URL:     srv.URL + "/tampered.zip",
		SHA256:  sum,
	}
	manifestData, err := manifestDataForSignature(entry)
	if err != nil {
		t.Fatalf("manifestDataForSignature: %v", err)
	}
	sig, err := Sign(manifestData, privPEM)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	entry.Signature = Signature{PublicKeyID: "market-key", Signature: sig}
	// 篡改条目内容，使签名不再匹配。
	entry.Description = "tampered description"

	ts := NewTrustStore()
	if err := ts.AddPublicKey("market-key", pubPEM); err != nil {
		t.Fatalf("AddPublicKey: %v", err)
	}

	dest := t.TempDir()
	_, err = InstallFromMarket(context.Background(), []MarketIndex{entry}, "tampered", dest, ts)
	if err == nil {
		t.Fatal("expected InstallFromMarket to reject tampered entry")
	}
	if !strings.Contains(err.Error(), "signature verification failed") {
		t.Fatalf("expected signature verification failure, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dest, "tampered")); !os.IsNotExist(statErr) {
		t.Fatalf("plugin directory should not be created after signature failure: %v", statErr)
	}
}

func TestInstallFromMarketRequiresSignatureWhenTrustStoreNonEmpty(t *testing.T) {
	_, pubPEM := newSignatureTestKey(t)
	payload, sum := makePluginZip(t, "unsigned", "1.0.0")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	entry := MarketIndex{
		Name:    "unsigned",
		Version: "1.0.0",
		URL:     srv.URL + "/unsigned.zip",
		SHA256:  sum,
	}

	ts := NewTrustStore()
	if err := ts.AddPublicKey("market-key", pubPEM); err != nil {
		t.Fatalf("AddPublicKey: %v", err)
	}

	dest := t.TempDir()
	_, err := InstallFromMarket(context.Background(), []MarketIndex{entry}, "unsigned", dest, ts)
	if err == nil {
		t.Fatal("expected InstallFromMarket to require signature")
	}
	if !errors.Is(err, ErrMissingSignature) {
		t.Fatalf("expected ErrMissingSignature, got %v", err)
	}
}
