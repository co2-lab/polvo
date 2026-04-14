package secrets

import (
	"strings"
	"testing"
)

func TestMaskSecrets_AWSKeys(t *testing.T) {
	t.Run("AKIA bare key", func(t *testing.T) {
		input := "Access key: AKIAIOSFODNN7EXAMPLE"
		masked, count := MaskSecrets(input)
		if count == 0 {
			t.Error("expected at least 1 match")
		}
		if strings.Contains(masked, "AKIAIOSFODNN7EXAMPLE") {
			t.Errorf("AKIA key not redacted: %q", masked)
		}
		if !strings.Contains(masked, "[AWS_ACCESS_KEY_REDACTED]") {
			t.Errorf("expected [AWS_ACCESS_KEY_REDACTED] in output: %q", masked)
		}
	})

	t.Run("aws_access_key_id assignment", func(t *testing.T) {
		input := "AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE"
		masked, count := MaskSecrets(input)
		if count == 0 {
			t.Error("expected at least 1 match")
		}
		if strings.Contains(masked, "AKIAIOSFODNN7EXAMPLE") {
			t.Errorf("AWS key not redacted: %q", masked)
		}
	})

	t.Run("aws_secret_access_key assignment", func(t *testing.T) {
		input := "aws_secret_access_key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
		masked, count := MaskSecrets(input)
		if count == 0 {
			t.Error("expected at least 1 match")
		}
		if strings.Contains(masked, "wJalrXUtnFEMI") {
			t.Errorf("AWS secret not redacted: %q", masked)
		}
	})
}

func TestMaskSecrets_OpenAIKey(t *testing.T) {
	// sk- followed by exactly 48 alphanumeric characters
	key := "sk-" + strings.Repeat("a", 48)
	input := "my key is " + key
	masked, count := MaskSecrets(input)
	if count == 0 {
		t.Error("expected 1 match")
	}
	if strings.Contains(masked, key) {
		t.Errorf("OpenAI key not redacted: %q", masked)
	}
	if !strings.Contains(masked, "[OPENAI_KEY_REDACTED]") {
		t.Errorf("expected [OPENAI_KEY_REDACTED] in output: %q", masked)
	}
}

func TestMaskSecrets_GitHubPAT(t *testing.T) {
	// ghp_ followed by exactly 36 alphanumeric characters
	token := "ghp_" + strings.Repeat("A", 36)
	input := "token: " + token
	masked, count := MaskSecrets(input)
	if count == 0 {
		t.Error("expected 1 match")
	}
	if strings.Contains(masked, token) {
		t.Errorf("GitHub PAT not redacted: %q", masked)
	}
	if !strings.Contains(masked, "[GITHUB_PAT_REDACTED]") {
		t.Errorf("expected [GITHUB_PAT_REDACTED] in output: %q", masked)
	}
}

func TestMaskSecrets_GitHubOAuth(t *testing.T) {
	token := "gho_" + strings.Repeat("B", 36)
	input := "oauth=" + token
	masked, count := MaskSecrets(input)
	if count == 0 {
		t.Error("expected 1 match")
	}
	if strings.Contains(masked, token) {
		t.Errorf("GitHub OAuth token not redacted: %q", masked)
	}
	if !strings.Contains(masked, "[GITHUB_OAUTH_REDACTED]") {
		t.Errorf("expected [GITHUB_OAUTH_REDACTED] in output: %q", masked)
	}
}

func TestMaskSecrets_SlackToken(t *testing.T) {
	input := "xoxb-" + "12345678-AbCdEfGhIjKlMnOp" // split to avoid secret scanning false positives
	masked, count := MaskSecrets(input)
	if count == 0 {
		t.Error("expected 1 match")
	}
	if strings.Contains(masked, "xoxb-12345678") {
		t.Errorf("Slack token not redacted: %q", masked)
	}
	if !strings.Contains(masked, "[SLACK_TOKEN_REDACTED]") {
		t.Errorf("expected [SLACK_TOKEN_REDACTED] in output: %q", masked)
	}
}

func TestMaskSecrets_APIKey(t *testing.T) {
	t.Run("api_key assignment", func(t *testing.T) {
		input := "api_key=mysecretkey123"
		masked, count := MaskSecrets(input)
		if count == 0 {
			t.Error("expected 1 match")
		}
		if strings.Contains(masked, "mysecretkey123") {
			t.Errorf("API key not redacted: %q", masked)
		}
		if !strings.Contains(masked, "[API_KEY_REDACTED]") {
			t.Errorf("expected [API_KEY_REDACTED] in output: %q", masked)
		}
	})

	t.Run("apikey colon assignment", func(t *testing.T) {
		input := "apikey: supersecret"
		masked, count := MaskSecrets(input)
		if count == 0 {
			t.Error("expected 1 match")
		}
		if strings.Contains(masked, "supersecret") {
			t.Errorf("apikey not redacted: %q", masked)
		}
	})

	t.Run("api-key dash form", func(t *testing.T) {
		input := "api-key=anothersecret"
		masked, count := MaskSecrets(input)
		if count == 0 {
			t.Error("expected 1 match")
		}
		if strings.Contains(masked, "anothersecret") {
			t.Errorf("api-key not redacted: %q", masked)
		}
	})
}

func TestMaskSecrets_Password(t *testing.T) {
	t.Run("password= form", func(t *testing.T) {
		input := "password=hunter2"
		masked, count := MaskSecrets(input)
		if count == 0 {
			t.Error("expected 1 match")
		}
		if strings.Contains(masked, "hunter2") {
			t.Errorf("password not redacted: %q", masked)
		}
		if !strings.Contains(masked, "[PASSWORD_REDACTED]") {
			t.Errorf("expected [PASSWORD_REDACTED] in output: %q", masked)
		}
	})

	t.Run("passwd colon form", func(t *testing.T) {
		input := "passwd: secret123"
		masked, count := MaskSecrets(input)
		if count == 0 {
			t.Error("expected 1 match")
		}
		if strings.Contains(masked, "secret123") {
			t.Errorf("passwd not redacted: %q", masked)
		}
	})

	t.Run("pwd= form", func(t *testing.T) {
		input := "pwd=mypassword"
		masked, count := MaskSecrets(input)
		if count == 0 {
			t.Error("expected 1 match")
		}
		if strings.Contains(masked, "mypassword") {
			t.Errorf("pwd not redacted: %q", masked)
		}
	})
}

func TestMaskSecrets_Token(t *testing.T) {
	// token value must be >= 20 chars
	input := "token=abcdefghijklmnopqrstuvwxyz"
	masked, count := MaskSecrets(input)
	if count == 0 {
		t.Error("expected 1 match")
	}
	if strings.Contains(masked, "abcdefghijklmnopqrstuvwxyz") {
		t.Errorf("token not redacted: %q", masked)
	}
	if !strings.Contains(masked, "[TOKEN_REDACTED]") {
		t.Errorf("expected [TOKEN_REDACTED] in output: %q", masked)
	}
}

func TestMaskSecrets_Authorization(t *testing.T) {
	input := "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"
	masked, count := MaskSecrets(input)
	if count == 0 {
		t.Error("expected 1 match")
	}
	if strings.Contains(masked, "Bearer eyJ") {
		t.Errorf("Authorization header not redacted: %q", masked)
	}
	if !strings.Contains(masked, "[AUTH_REDACTED]") {
		t.Errorf("expected [AUTH_REDACTED] in output: %q", masked)
	}
}

func TestMaskSecrets_NoFalsePositives(t *testing.T) {
	cases := []string{
		"This is a normal sentence without secrets",
		"hello world foo bar",
		"package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"hello\") }",
		"The total count is 42",
		"config.yaml loaded successfully",
	}
	for _, c := range cases {
		masked, count := MaskSecrets(c)
		if count != 0 {
			t.Errorf("false positive for %q: count=%d, masked=%q", c, count, masked)
		}
		if masked != c {
			t.Errorf("content changed without secrets for %q: got %q", c, masked)
		}
	}
}

func TestMaskSecrets_MultipleSecrets(t *testing.T) {
	input := "password=hunter2 and api_key=myrandomapikey123"
	_, count := MaskSecrets(input)
	if count < 2 {
		t.Errorf("expected at least 2 matches, got %d", count)
	}
}

func TestMaskSecrets_EmptyInput(t *testing.T) {
	masked, count := MaskSecrets("")
	if count != 0 {
		t.Errorf("expected 0 matches for empty string, got %d", count)
	}
	if masked != "" {
		t.Errorf("expected empty string, got %q", masked)
	}
}

func TestMaskSecrets_TokenTooShort(t *testing.T) {
	// token value shorter than 20 chars should NOT be redacted
	input := "token=shortvalue"
	masked, count := MaskSecrets(input)
	if count != 0 {
		t.Errorf("expected 0 matches for short token value, got %d (masked: %q)", count, masked)
	}
}

// ---------------------------------------------------------------------------
// Plan 45 tests — new patterns
// ---------------------------------------------------------------------------

func TestMaskSecrets_JWT(t *testing.T) {
	// JWT bare token
	input := "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"
	masked, count := MaskSecrets(input)
	if count == 0 {
		t.Error("expected JWT to be masked")
	}
	if strings.Contains(masked, "eyJhbGci") {
		t.Errorf("JWT not redacted: %q", masked)
	}
	if !strings.Contains(masked, "[JWT_REDACTED]") {
		t.Errorf("expected [JWT_REDACTED] in output: %q", masked)
	}
}

func TestMaskSecrets_PrivateKey(t *testing.T) {
	input := "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA...\n-----END RSA PRIVATE KEY-----"
	masked, count := MaskSecrets(input)
	if count == 0 {
		t.Error("expected private key header to be masked")
	}
	if strings.Contains(masked, "BEGIN RSA PRIVATE KEY") {
		t.Errorf("private key not redacted: %q", masked)
	}
	if !strings.Contains(masked, "[PRIVATE_KEY_REDACTED]") {
		t.Errorf("expected [PRIVATE_KEY_REDACTED] in output: %q", masked)
	}
}

func TestMaskSecrets_DBConnString(t *testing.T) {
	input := "postgres://admin:s3cr3tp4ss@db.example.com:5432/prod"
	masked, count := MaskSecrets(input)
	if count == 0 {
		t.Error("expected DB connection string to be masked")
	}
	if strings.Contains(masked, "s3cr3tp4ss") {
		t.Errorf("DB conn string not redacted: %q", masked)
	}
	if !strings.Contains(masked, "[DB_CONN_REDACTED]") {
		t.Errorf("expected [DB_CONN_REDACTED] in output: %q", masked)
	}
}

func TestMaskSecrets_StripeLiveKey(t *testing.T) {
	input := "sk_live_" + "AbCdEfGhIjKlMnOpQrStUvWxYz1234" // split to avoid secret scanning false positives
	masked, count := MaskSecrets(input)
	if count == 0 {
		t.Error("expected Stripe live key to be masked")
	}
	if strings.Contains(masked, "sk_live_") {
		t.Errorf("Stripe key not redacted: %q", masked)
	}
	if !strings.Contains(masked, "[STRIPE_KEY_REDACTED]") {
		t.Errorf("expected [STRIPE_KEY_REDACTED] in output: %q", masked)
	}
}

func TestMaskSecrets_BearerStandalone(t *testing.T) {
	// Bearer without "authorization:" prefix
	input := "Bearer abcdefghijklmnopqrstuvwxyz1234"
	masked, _ := MaskSecrets(input)
	if strings.Contains(masked, "abcdefghijklmnopqrstuvwxyz1234") {
		t.Errorf("Bearer token not redacted: %q", masked)
	}
}

// ---------------------------------------------------------------------------
// Shannon entropy tests
// ---------------------------------------------------------------------------

func TestShannonEntropy(t *testing.T) {
	t.Run("uniform hex", func(t *testing.T) {
		// 16 distinct hex chars — high entropy
		h := ShannonEntropy("0123456789abcdef")
		if h < 3.9 {
			t.Errorf("expected entropy ~4.0, got %f", h)
		}
	})

	t.Run("all same char", func(t *testing.T) {
		h := ShannonEntropy("aaaaaaaaaaaaaaaa")
		if h != 0.0 {
			t.Errorf("expected entropy 0.0, got %f", h)
		}
	})

	t.Run("empty", func(t *testing.T) {
		h := ShannonEntropy("")
		if h != 0.0 {
			t.Errorf("expected 0 for empty string, got %f", h)
		}
	})
}

func TestIsHighEntropySecret(t *testing.T) {
	t.Run("AWS secret key (base64-like)", func(t *testing.T) {
		// AWS secret key format: alphanumeric mixed
		key := "wJalrXUtnFEMI7K7MDENGbPxRfiCYEXAMPLEKEY"
		if !isHighEntropySecret(key) {
			t.Errorf("expected high entropy for AWS-like key %q", key)
		}
	})

	t.Run("hex short — too short", func(t *testing.T) {
		if isHighEntropySecret("deadbeef") {
			t.Error("short hex should not be flagged")
		}
	})

	t.Run("base64 low entropy — just test encoded", func(t *testing.T) {
		// "test" in base64 is "dGVzdA==" — low entropy
		if isHighEntropySecret("dGVzdA==") {
			t.Error("low-entropy base64 should not be flagged")
		}
	})

	t.Run("hex random 32 chars", func(t *testing.T) {
		if !isHighEntropySecret("5f4dcc3b5aa765d61d8327deb882cf99") {
			t.Error("expected high entropy for 32-char hex")
		}
	})

	t.Run("UUID with hyphens — not classified", func(t *testing.T) {
		// Hyphens prevent hex/base64/alphanum classification
		if isHighEntropySecret("550e8400-e29b-41d4-a716-446655440000") {
			t.Error("UUID with hyphens should not be flagged")
		}
	})
}

// ---------------------------------------------------------------------------
// False positives — must NOT be masked
// ---------------------------------------------------------------------------

func TestMaskSecrets_FalsePositivesPlan45(t *testing.T) {
	cases := []string{
		"version: 1.2.3-beta",
		"2026-04-13T10:00:00Z",
		"fmt.Sprintf(\"%s\", val)",
		"https://example.com/api/v1/users",
		"SELECT * FROM users WHERE id = ?",
		"0000000000000000",
		"func(ctx context.Context) error",
	}
	for _, c := range cases {
		masked, count := MaskSecrets(c)
		if count != 0 {
			t.Errorf("false positive for %q: count=%d, masked=%q", c, count, masked)
		}
	}
}

// ---------------------------------------------------------------------------
// MaskSecretsDetailed structural test
// ---------------------------------------------------------------------------

func TestMaskSecretsDetailed_Structure(t *testing.T) {
	token := "ghp_" + strings.Repeat("A", 36)
	result := MaskSecretsDetailed("token: " + token)
	if result.Masked == "token: "+token {
		t.Error("content should have been masked")
	}
	if len(result.Redactions) == 0 {
		t.Error("expected at least one redaction")
	}
}
