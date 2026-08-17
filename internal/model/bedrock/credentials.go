// Package bedrock loads standard AWS credentials and signs direct Bedrock
// runtime requests. It does not invoke the AWS CLI or delegate Gator's loop.
package bedrock

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
)

const service = "bedrock"

// Signer retains AWS's refreshable credential provider and signs every
// request at the direct Bedrock runtime boundary.
type Signer struct {
	credentials aws.CredentialsProvider
	region      string
	now         func() time.Time
}

// LoadDefault constructs the AWS SDK's standard ambient credential chain. The
// chain includes static environment values, web identity, shared profiles, ECS
// task credentials, and EC2 instance metadata without starting the AWS CLI.
func LoadDefault(ctx context.Context, client *http.Client) (Signer, error) {
	options := make([]func(*awsconfig.LoadOptions) error, 0, 1)
	if client != nil {
		options = append(options, awsconfig.WithHTTPClient(client))
	}
	configured, err := awsconfig.LoadDefaultConfig(ctx, options...)
	if err != nil {
		return Signer{}, err
	}
	if configured.Credentials == nil {
		return Signer{}, errors.New("AWS credential chain is not configured")
	}
	region := strings.TrimSpace(configured.Region)
	if region == "" {
		region = "us-east-1"
	}
	return Signer{credentials: configured.Credentials, region: region, now: time.Now}, nil
}

// SignRequest resolves refreshable credentials and applies AWS Signature V4
// for the direct Bedrock service. It deliberately replaces any pre-existing
// Authorization header so no bearer credential can be sent alongside SigV4.
func (s Signer) SignRequest(ctx context.Context, request *http.Request, payload []byte) error {
	if s.credentials == nil {
		return errors.New("AWS credential chain is not configured")
	}
	credentials, err := s.credentials.Retrieve(ctx)
	if err != nil {
		return err
	}
	request.Header.Del("Authorization")
	digest := sha256.Sum256(payload)
	now := s.now
	if now == nil {
		now = time.Now
	}
	return v4.NewSigner().SignHTTP(ctx, credentials, request, hex.EncodeToString(digest[:]), service, s.region, now())
}

// Region reports the region resolved from the standard AWS configuration.
func (s Signer) Region() string {
	return s.region
}

// AmbientSource reports a configured source without reading a credential's
// value or contacting AWS. It is suitable for diagnostics only.
func AmbientSource() (string, bool) {
	if strings.TrimSpace(os.Getenv("AWS_ACCESS_KEY_ID")) != "" && strings.TrimSpace(os.Getenv("AWS_SECRET_ACCESS_KEY")) != "" {
		return "AWS static credentials in environment", true
	}
	if strings.TrimSpace(os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE")) != "" && strings.TrimSpace(os.Getenv("AWS_ROLE_ARN")) != "" {
		return "AWS web identity credentials", true
	}
	if strings.TrimSpace(os.Getenv("AWS_CONTAINER_CREDENTIALS_FULL_URI")) != "" || strings.TrimSpace(os.Getenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI")) != "" {
		return "AWS ECS task credentials", true
	}
	if sharedConfigExists() {
		return "AWS shared profile", true
	}
	return "", false
}

func sharedConfigExists() bool {
	for _, variable := range []string{"AWS_SHARED_CREDENTIALS_FILE", "AWS_CONFIG_FILE"} {
		if path := strings.TrimSpace(os.Getenv(variable)); path != "" {
			if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
				return true
			}
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	for _, name := range []string{"credentials", "config"} {
		if info, err := os.Stat(filepath.Join(home, ".aws", name)); err == nil && info.Mode().IsRegular() {
			return true
		}
	}
	return false
}
