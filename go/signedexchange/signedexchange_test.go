package signedexchange_test

import (
	"bytes"
	"encoding/pem"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"testing"
	"time"

	. "github.com/WICG/webpackage/go/signedexchange"
	"github.com/WICG/webpackage/go/signedexchange/certurl"
	"github.com/WICG/webpackage/go/signedexchange/version"
)

const (
	requestUrl = "https://example.com/"

	payload  = `Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.`
	pemCerts = `-----BEGIN CERTIFICATE-----
MIIBhjCCAS2gAwIBAgIJAOhR3xtYd5QsMAoGCCqGSM49BAMCMDIxFDASBgNVBAMM
C2V4YW1wbGUub3JnMQ0wCwYDVQQKDARUZXN0MQswCQYDVQQGEwJVUzAeFw0xODEx
MDUwOTA5MjJaFw0xOTEwMzEwOTA5MjJaMDIxFDASBgNVBAMMC2V4YW1wbGUub3Jn
MQ0wCwYDVQQKDARUZXN0MQswCQYDVQQGEwJVUzBZMBMGByqGSM49AgEGCCqGSM49
AwEHA0IABH1E6odXRm3+r7dMYmkJRmftx5IYHAsqgA7zjsFfCvPqL/fM4Uvi8EFu
JVQM/oKEZw3foCZ1KBjo/6Tenkoj/wCjLDAqMBAGCisGAQQB1nkCARYEAgUAMBYG
A1UdEQQPMA2CC2V4YW1wbGUub3JnMAoGCCqGSM49BAMCA0cAMEQCIEbxRKhlQYlw
Ja+O9h7misjLil82Q82nhOtl4j96awZgAiB6xrvRZIlMtWYKdi41BTb5fX22gL9M
L/twWg8eWpYeJA==
-----END CERTIFICATE-----
`
	// Generated by `openssl ecparam -out priv.key -name prime256v1 -genkey`
	pemPrivateKey = `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIEMac81NMjwO4pQ2IGKZ3UdymYtnFAXEjKdvAdEx4DQwoAoGCCqGSM49
AwEHoUQDQgAEfUTqh1dGbf6vt0xiaQlGZ+3HkhgcCyqADvOOwV8K8+ov98zhS+Lw
QW4lVAz+goRnDd+gJnUoGOj/pN6eSiP/AA==
-----END EC PRIVATE KEY-----`
)

// signatureDate corresponds to the expectedSignatureHeader's date value.
var signatureDate = time.Date(2018, 1, 31, 17, 13, 20, 0, time.UTC)

var nullLogger = log.New(ioutil.Discard, "", 0)     // Use when some output is expected.
var stdoutLogger = log.New(os.Stdout, "ERROR: ", 0) // Use when no output is expected.

type zeroReader struct{}

func (zeroReader) Read(b []byte) (int, error) {
	for i := range b {
		b[i] = 0
	}
	return len(b), nil
}

func mustReadFile(path string) []byte {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return b
}

func testForEachVersion(t *testing.T, testFunc func(ver version.Version, t *testing.T)) {
	for _, ver := range version.AllVersions {
		t.Run(string(ver), func(t *testing.T) { testFunc(ver, t) })
	}
}

func TestSignedExchange(t *testing.T) {
	certs, err := ParseCertificates([]byte(pemCerts))
	if err != nil {
		t.Fatal(err)
	}

	derPrivateKey, _ := pem.Decode([]byte(pemPrivateKey))
	privKey, err := ParsePrivateKey(derPrivateKey.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	certUrl, _ := url.Parse("https://example.com/cert.msg")
	validityUrl, _ := url.Parse("https://example.com/resource.validity")

	expectedRespHeader := map[version.Version]http.Header{
		version.Version1b1: http.Header{
			"Content-Type":     []string{"text/html; charset=utf-8"},
			"Foo":              []string{"Bar,Baz"},
			"Content-Encoding": []string{"mi-sha256-draft2"},
			"Mi-Draft2":        []string{"mi-sha256-draft2=DRyBGPb7CAW2ukzb9sT1S1ialssthiv6QW7Ks-Trg4Y"},
		},
		version.Version1b2: http.Header{
			"Content-Type":     []string{"text/html; charset=utf-8"},
			"Foo":              []string{"Bar,Baz"},
			"Content-Encoding": []string{"mi-sha256-03"},
			"Digest":           []string{"mi-sha256-03=DRyBGPb7CAW2ukzb9sT1S1ialssthiv6QW7Ks+Trg4Y="},
		},
		version.Version1b3: http.Header{
			"Content-Type":     []string{"text/html; charset=utf-8"},
			"Foo":              []string{"Bar,Baz"},
			"Content-Encoding": []string{"mi-sha256-03"},
			"Digest":           []string{"mi-sha256-03=DRyBGPb7CAW2ukzb9sT1S1ialssthiv6QW7Ks+Trg4Y="},
		},
	}
	expectedSignatureHeader := map[version.Version]string{
		version.Version1b1: "label; sig=*MEYCIQCbay5VbkR9mi4pnwDAJamuf7Fj1CWnEnJt6Uxm7YeqiwIhAL8JISyzF5sDhtUaEbNCE6vgv2NIKCkONzLgwL23UL6P*; validity-url=\"https://example.com/resource.validity\"; integrity=\"mi-draft2\"; cert-url=\"https://example.com/cert.msg\"; cert-sha256=*eLWHusI0YcDcHSG5nkYbyZddE2sidVyhx6iSYoJ+SFc=*; date=1517418800; expires=1517422400",
		version.Version1b2: "label; sig=*MEUCIHNiDRQncQpVxW2x+woinMUTY8nuSQfi0mbJ5J6x7FZyAiEAgh6FH6PdncNCK8GHTwN3wfUUUFdjVswNi1PfIgCOwHk=*; validity-url=\"https://example.com/resource.validity\"; integrity=\"digest/mi-sha256-03\"; cert-url=\"https://example.com/cert.msg\"; cert-sha256=*eLWHusI0YcDcHSG5nkYbyZddE2sidVyhx6iSYoJ+SFc=*; date=1517418800; expires=1517422400",
		version.Version1b3: "label; sig=*MEUCIEQPK0UKPm9/XP5Jko2V72vTrGlBqB9HHoOzhJmVPflmAiEAwCSBw98NhUhFGJaxL6ITT+QZBBeO7TCLAiHn1apY6Es=*; validity-url=\"https://example.com/resource.validity\"; integrity=\"digest/mi-sha256-03\"; cert-url=\"https://example.com/cert.msg\"; cert-sha256=*eLWHusI0YcDcHSG5nkYbyZddE2sidVyhx6iSYoJ+SFc=*; date=1517418800; expires=1517422400",
	}

	testForEachVersion(t, func(ver version.Version, t *testing.T) {
		reqHeader := http.Header{}
		reqHeader.Add("Accept", "*/*")
		respHeader := http.Header{}
		respHeader.Add("Content-Type", "text/html; charset=utf-8")

		// Multiple values for the same header
		respHeader.Add("Foo", "Bar")
		respHeader.Add("Foo", "Baz")

		e := NewExchange(ver, requestUrl, http.MethodGet, reqHeader, 200, respHeader, []byte(payload))
		if err := e.MiEncodePayload(16); err != nil {
			t.Fatal(err)
		}

		s := &Signer{
			Date:        signatureDate,
			Expires:     signatureDate.Add(1 * time.Hour),
			Certs:       certs,
			CertUrl:     certUrl,
			ValidityUrl: validityUrl,
			PrivKey:     privKey,
			Rand:        zeroReader{},
		}
		if err := e.AddSignatureHeader(s); err != nil {
			t.Fatal(err)
		}

		var buf bytes.Buffer
		if err := e.Write(&buf); err != nil {
			t.Fatal(err)
		}

		got, err := ReadExchange(&buf)
		if err != nil {
			t.Fatal(err)
		}

		if got.Version != ver {
			t.Errorf("Unexpected version: got %v, want %v", got.Version, ver)
		}

		if got.RequestURI != requestUrl {
			t.Errorf("Unexpected request URL: %q", got.RequestURI)
		}

		if got.RequestMethod != http.MethodGet {
			t.Errorf("Unexpected request method: %q", got.RequestMethod)
		}

		if ver == version.Version1b1 || ver == version.Version1b2 {
			if !reflect.DeepEqual(got.RequestHeaders, reqHeader) {
				t.Errorf("Unexpected request headers: %v", got.RequestHeaders)
			}
		} else {
			emptyHeader := http.Header{}
			if !reflect.DeepEqual(got.RequestHeaders, emptyHeader) {
				t.Errorf("Unexpected request headers: %v", got.RequestHeaders)
			}
		}

		if got.ResponseStatus != 200 {
			t.Errorf("Unexpected response status: %v", got.ResponseStatus)
		}

		if !reflect.DeepEqual(got.ResponseHeaders, expectedRespHeader[ver]) {
			t.Errorf("Unexpected response headers: %v", got.ResponseHeaders)
		}

		if got.SignatureHeaderValue != expectedSignatureHeader[ver] {
			t.Errorf("Unexpected signature header: %q", got.SignatureHeaderValue)
		}

		wantPayload := mustReadFile("test-signedexchange-expected-payload-mi.bin")
		if !bytes.Equal(got.Payload, wantPayload) {
			t.Errorf("payload mismatch")
		}
	})
}

func TestSignedExchangeBannedCertUrlScheme(t *testing.T) {
	testForEachVersion(t, func(ver version.Version, t *testing.T) {
		e := NewExchange(ver, requestUrl, http.MethodGet, nil, 200, http.Header{}, []byte(payload))
		if err := e.MiEncodePayload(16); err != nil {
			t.Fatal(err)
		}

		certs, _ := ParseCertificates([]byte(pemCerts))
		certUrl, _ := url.Parse("http://example.com/cert.msg")
		validityUrl, _ := url.Parse("https://example.com/resource.validity")
		derPrivateKey, _ := pem.Decode([]byte(pemPrivateKey))
		privKey, _ := ParsePrivateKey(derPrivateKey.Bytes)
		s := &Signer{
			Date:        signatureDate,
			Expires:     signatureDate.Add(1 * time.Hour),
			Certs:       certs,
			CertUrl:     certUrl,
			ValidityUrl: validityUrl,
			PrivKey:     privKey,
			Rand:        zeroReader{},
		}
		if err := e.AddSignatureHeader(s); err == nil {
			t.Errorf("non-{https,data} cert-url unexpectedly allowed in an exchange")
		}
	})
}

func createTestExchange(ver version.Version, t *testing.T) (e *Exchange, s *Signer, certBytes []byte) {
	header := http.Header{}
	header.Add("Content-Type", "text/html; charset=utf-8")

	e = NewExchange(ver, requestUrl, http.MethodGet, nil, 200, header, []byte(payload))
	if err := e.MiEncodePayload(16); err != nil {
		t.Fatal(err)
	}

	certs, err := ParseCertificates([]byte(pemCerts))
	if err != nil {
		t.Fatal(err)
	}

	derPrivateKey, _ := pem.Decode([]byte(pemPrivateKey))
	privKey, err := ParsePrivateKey(derPrivateKey.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	certUrl, _ := url.Parse("https://example.com/cert.msg")
	validityUrl, _ := url.Parse("https://example.com/resource.validity")
	s = &Signer{
		Date:        signatureDate,
		Expires:     signatureDate.Add(1 * time.Hour),
		Certs:       certs,
		CertUrl:     certUrl,
		ValidityUrl: validityUrl,
		PrivKey:     privKey,
		Rand:        zeroReader{},
	}

	certChain, err := certurl.NewCertChain(certs, []byte("dummy"), nil)
	if err != nil {
		t.Fatal(err)
	}
	var certCBOR bytes.Buffer
	if err := certChain.Write(&certCBOR); err != nil {
		t.Fatal(err)
	}
	certBytes = certCBOR.Bytes()
	return
}

func TestVerify(t *testing.T) {
	testForEachVersion(t, func(ver version.Version, t *testing.T) {
		e, s, c := createTestExchange(ver, t)
		if err := e.AddSignatureHeader(s); err != nil {
			t.Fatal(err)
		}
		certFetcher := func(_ string) ([]byte, error) { return c, nil }

		verificationTime := signatureDate
		if _, ok := e.Verify(verificationTime, certFetcher, stdoutLogger); !ok {
			t.Errorf("Verification failed")
		}
	})
}

func TestVerifyNotYetValidExchange(t *testing.T) {
	testForEachVersion(t, func(ver version.Version, t *testing.T) {
		e, s, c := createTestExchange(ver, t)
		if err := e.AddSignatureHeader(s); err != nil {
			t.Fatal(err)
		}
		certFetcher := func(_ string) ([]byte, error) { return c, nil }

		verificationTime := signatureDate.Add(-1 * time.Second)
		if _, ok := e.Verify(verificationTime, certFetcher, nullLogger); ok {
			t.Errorf("Verification should fail")
		}
	})
}

func TestVerifyExpiredExchange(t *testing.T) {
	testForEachVersion(t, func(ver version.Version, t *testing.T) {
		e, s, c := createTestExchange(ver, t)
		if err := e.AddSignatureHeader(s); err != nil {
			t.Fatal(err)
		}
		certFetcher := func(_ string) ([]byte, error) { return c, nil }

		verificationTime := signatureDate.Add(1 * time.Hour).Add(1 * time.Second)
		if _, ok := e.Verify(verificationTime, certFetcher, nullLogger); ok {
			t.Errorf("Verification should fail")
		}
	})
}

func TestVerifyBadValidityUrl(t *testing.T) {
	testForEachVersion(t, func(ver version.Version, t *testing.T) {
		e, s, c := createTestExchange(ver, t)
		s.ValidityUrl, _ = url.Parse("https://subdomain.example.com/resource.validity")
		if err := e.AddSignatureHeader(s); err != nil {
			t.Fatal(err)
		}
		certFetcher := func(_ string) ([]byte, error) { return c, nil }

		verificationTime := signatureDate
		if _, ok := e.Verify(verificationTime, certFetcher, nullLogger); ok {
			t.Errorf("Verification should fail")
		}
	})
}

func TestVerifyBadMethod(t *testing.T) {
	testForEachVersion(t, func(ver version.Version, t *testing.T) {
		// The test doesn't make sense in version >= b3, which doesn't have request method.
		if ver != version.Version1b1 && ver != version.Version1b2 {
			return
		}

		e, s, c := createTestExchange(ver, t)
		e.RequestMethod = "POST"
		if err := e.AddSignatureHeader(s); err != nil {
			t.Fatal(err)
		}
		certFetcher := func(_ string) ([]byte, error) { return c, nil }

		verificationTime := signatureDate
		if _, ok := e.Verify(verificationTime, certFetcher, nullLogger); ok {
			t.Errorf("Verification should fail")
		}
	})
}

func TestVerifyStatefulRequestHeader(t *testing.T) {
	testForEachVersion(t, func(ver version.Version, t *testing.T) {
		e, s, c := createTestExchange(ver, t)
		if e.RequestHeaders == nil {
			e.RequestHeaders = http.Header{}
		}
		e.RequestHeaders.Set("Authorization", "Basic Zm9vOmJhcg==")
		if err := e.AddSignatureHeader(s); err != nil {
			t.Fatal(err)
		}
		certFetcher := func(_ string) ([]byte, error) { return c, nil }

		verificationTime := signatureDate
		if _, ok := e.Verify(verificationTime, certFetcher, nullLogger); ok {
			t.Errorf("Verification should fail")
		}
	})
}

func TestVerifyUncachedHeader(t *testing.T) {
	testForEachVersion(t, func(ver version.Version, t *testing.T) {
		e, s, c := createTestExchange(ver, t)
		e.ResponseHeaders.Set("Set-Cookie", "foo=bar")
		if err := e.AddSignatureHeader(s); err != nil {
			t.Fatal(err)
		}
		certFetcher := func(_ string) ([]byte, error) { return c, nil }

		verificationTime := signatureDate
		if _, ok := e.Verify(verificationTime, certFetcher, nullLogger); ok {
			t.Errorf("Verification should fail")
		}
	})
}

func TestVerifyBadSignature(t *testing.T) {
	testForEachVersion(t, func(ver version.Version, t *testing.T) {
		e, s, c := createTestExchange(ver, t)
		if err := e.AddSignatureHeader(s); err != nil {
			t.Fatal(err)
		}
		certFetcher := func(_ string) ([]byte, error) { return c, nil }

		e.ResponseHeaders.Add("Etag", "0123")

		verificationTime := signatureDate
		if _, ok := e.Verify(verificationTime, certFetcher, nullLogger); ok {
			t.Errorf("Verification should fail")
		}
	})
}

func TestVerifyNoContentType(t *testing.T) {
	testForEachVersion(t, func(ver version.Version, t *testing.T) {
		e, s, c := createTestExchange(ver, t)
		e.ResponseHeaders.Del("Content-Type");
		if err := e.AddSignatureHeader(s); err != nil {
			t.Fatal(err)
		}
		certFetcher := func(_ string) ([]byte, error) { return c, nil }
		verificationTime := signatureDate

		// The requirement for Content-Type is only for version >= b3.
		if ver == version.Version1b1 || ver == version.Version1b2 {
			if _, ok := e.Verify(verificationTime, certFetcher, stdoutLogger); !ok {
				t.Errorf("Verification should succeed")
			}
		} else {
			if _, ok := e.Verify(verificationTime, certFetcher, nullLogger); ok {
				t.Errorf("Verification should fail")
			}
		}
	})
}

func TestVerifyNonCanonicalURL(t *testing.T) {
	testForEachVersion(t, func(ver version.Version, t *testing.T) {
		e, s, c := createTestExchange(ver, t)
		// url.Parse() decodes "%73%78%67" to "sxg"
		e.RequestURI = "https://example.com/%73%78%67"
		if err := e.AddSignatureHeader(s); err != nil {
			t.Fatal(err)
		}
		certFetcher := func(_ string) ([]byte, error) { return c, nil }

		verificationTime := signatureDate
		if _, ok := e.Verify(verificationTime, certFetcher, stdoutLogger); !ok {
			t.Errorf("Verification failed")
		}
	})
}

func TestVerifyNonCacheable(t *testing.T) {
	testForEachVersion(t, func(ver version.Version, t *testing.T) {
		e, s, c := createTestExchange(ver, t)
		e.ResponseHeaders.Add("Cache-Control", "no-store")
		if err := e.AddSignatureHeader(s); err != nil {
			t.Fatal(err)
		}
		certFetcher := func(_ string) ([]byte, error) { return c, nil }

		verificationTime := signatureDate
		switch ver {
		case version.Version1b1, version.Version1b2:
			if _, ok := e.Verify(verificationTime, certFetcher, stdoutLogger); !ok {
				t.Errorf("Verification should succeed")
			}
		default:
			if _, ok := e.Verify(verificationTime, certFetcher, nullLogger); ok {
				t.Errorf("Verification should fail")
			}
		}
	})
}

func TestIsCacheable(t *testing.T) {
	testForEachVersion(t, func(ver version.Version, t *testing.T) {
		if ver == version.Version1b1 || ver == version.Version1b2 {
			return
		}

		e, _, _ := createTestExchange(ver, t)
		if !e.IsCacheable(stdoutLogger) {
			t.Errorf("Response should be cacheable")
		}

		e, _, _ = createTestExchange(ver, t)
		e.ResponseHeaders.Add("cache-control", "no-store")
		if e.IsCacheable(nullLogger) {
			t.Errorf("Response with \"no-store\" cache directive shouldn't be cacheable")
		}

		e, _, _ = createTestExchange(ver, t)
		e.ResponseHeaders.Add("cache-control", "max-age=300, private")
		if e.IsCacheable(nullLogger) {
			t.Errorf("Response with \"private\" cache directive shouldn't be cacheable")
		}

		e, _, _ = createTestExchange(ver, t)
		e.ResponseStatus = 201
		if e.IsCacheable(nullLogger) {
			t.Errorf("Response with status code 201 shouldn't be cacheable by default")
		}

		e, _, _ = createTestExchange(ver, t)
		e.ResponseStatus = 201
		e.ResponseHeaders.Add("cache-control", "max-age=300")
		if !e.IsCacheable(stdoutLogger) {
			t.Errorf("Response with \"max-age\" cache directive should be cacheable")
		}

		e, _, _ = createTestExchange(ver, t)
		e.ResponseStatus = 201
		e.ResponseHeaders.Add("expires", "Mon, 07 Jan 2019 07:29:39 GMT")
		if !e.IsCacheable(stdoutLogger) {
			t.Errorf("Response with \"Expires\" header should be cacheable")
		}
	})
}
