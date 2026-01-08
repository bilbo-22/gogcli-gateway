package cmd

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/steipete/gogcli/internal/input"
	"github.com/steipete/gogcli/internal/tracking"
	"github.com/steipete/gogcli/internal/ui"
)

type GmailTrackSetupCmd struct {
	WorkerName   string `name:"worker-name" help:"Cloudflare Worker name (defaults to gog-email-tracker-<account>)"`
	DatabaseName string `name:"db-name" help:"D1 database name (defaults to worker name)"`
	WorkerURL    string `name:"worker-url" aliases:"domain" help:"Tracking worker base URL (e.g. https://gog-email-tracker.<acct>.workers.dev)"`
	TrackingKey  string `name:"tracking-key" help:"Tracking key (base64; generates one if omitted)"`
	AdminKey     string `name:"admin-key" help:"Admin key for /opens (generates one if omitted)"`
	Deploy       bool   `name:"deploy" help:"Provision D1 + deploy the worker (requires wrangler)"`
	WorkerDir    string `name:"worker-dir" help:"Worker directory (default: internal/tracking/worker)"`
}

func (c *GmailTrackSetupCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	cfg, err := tracking.LoadConfig(account)
	if err != nil {
		return fmt.Errorf("load tracking config: %w", err)
	}

	workerName := strings.TrimSpace(c.WorkerName)
	if workerName == "" {
		workerName = strings.TrimSpace(cfg.WorkerName)
	}
	if workerName == "" {
		workerName = defaultWorkerName(account)
	}
	workerName = sanitizeWorkerName(workerName)
	if workerName == "" {
		return fmt.Errorf("invalid worker name")
	}
	c.WorkerName = workerName

	dbName := strings.TrimSpace(c.DatabaseName)
	if dbName == "" {
		dbName = strings.TrimSpace(cfg.DatabaseName)
	}
	if dbName == "" {
		dbName = workerName
	}
	c.DatabaseName = dbName

	if c.WorkerURL == "" {
		c.WorkerURL = strings.TrimSpace(cfg.WorkerURL)
	}
	if c.WorkerURL == "" && !flags.NoInput {
		line, readErr := input.PromptLine(ctx, "Tracking worker base URL (e.g. https://...workers.dev): ")
		if readErr != nil {
			if errors.Is(readErr, io.EOF) || errors.Is(readErr, os.ErrClosed) {
				return &ExitError{Code: 1, Err: errors.New("cancelled")}
			}

			return fmt.Errorf("read worker url: %w", readErr)
		}

		c.WorkerURL = strings.TrimSpace(line)
	}
	c.WorkerURL = strings.TrimSpace(c.WorkerURL)
	if c.WorkerURL == "" {
		return usage("required: --worker-url")
	}

	key := strings.TrimSpace(c.TrackingKey)
	if key == "" {
		key = strings.TrimSpace(cfg.TrackingKey)
	}
	if key == "" {
		key, err = tracking.GenerateKey()
		if err != nil {
			return fmt.Errorf("generate tracking key: %w", err)
		}
	}

	adminKey := strings.TrimSpace(c.AdminKey)
	if adminKey == "" {
		adminKey = strings.TrimSpace(cfg.AdminKey)
	}
	if adminKey == "" {
		adminKey, err = generateAdminKey()
		if err != nil {
			return fmt.Errorf("generate admin key: %w", err)
		}
	}

	if err := tracking.SaveSecrets(account, key, adminKey); err != nil {
		return fmt.Errorf("save tracking secrets: %w", err)
	}

	cfg.Enabled = true
	cfg.WorkerURL = c.WorkerURL
	cfg.WorkerName = workerName
	cfg.DatabaseName = c.DatabaseName
	cfg.SecretsInKeyring = true
	cfg.TrackingKey = ""
	cfg.AdminKey = ""

	if c.WorkerDir == "" {
		c.WorkerDir = filepath.Join("internal", "tracking", "worker")
	}

	if c.Deploy {
		dbID, deployErr := deployTrackingWorker(ctx, u, c.WorkerDir, workerName, c.DatabaseName, key, adminKey)
		if deployErr != nil {
			return deployErr
		}
		cfg.DatabaseID = dbID
	}

	if err := tracking.SaveConfig(account, cfg); err != nil {
		return fmt.Errorf("save tracking config: %w", err)
	}

	path, _ := tracking.ConfigPath()
	u.Out().Printf("configured\ttrue")
	u.Out().Printf("account\t%s", account)
	if path != "" {
		u.Out().Printf("config_path\t%s", path)
	}
	u.Out().Printf("worker_url\t%s", cfg.WorkerURL)
	u.Out().Printf("worker_name\t%s", cfg.WorkerName)
	u.Out().Printf("database_name\t%s", cfg.DatabaseName)
	if cfg.DatabaseID != "" {
		u.Out().Printf("database_id\t%s", cfg.DatabaseID)
	}

	if !c.Deploy {
		u.Err().Println("")
		u.Err().Println("Next steps (manual worker deploy):")
		u.Err().Printf("  - cd %s", c.WorkerDir)
		u.Err().Println("  - use these values when prompted:")
		u.Err().Printf("    TRACKING_KEY=%s", key)
		u.Err().Printf("    ADMIN_KEY=%s", adminKey)
		u.Err().Printf("  - wrangler d1 create %s", c.DatabaseName)
		u.Err().Println("  - wrangler d1 execute <db> --file schema.sql --remote")
		u.Err().Printf("  - set wrangler.toml name=%s + database_id", cfg.WorkerName)
		u.Err().Println("  - wrangler secret put TRACKING_KEY")
		u.Err().Println("  - wrangler secret put ADMIN_KEY")
		u.Err().Println("  - wrangler deploy")
	}

	return nil
}

func generateAdminKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func defaultWorkerName(account string) string {
	sanitized := sanitizeWorkerName(account)
	if sanitized == "" {
		return "gog-email-tracker"
	}
	return "gog-email-tracker-" + sanitized
}

func sanitizeWorkerName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return ""
	}
	re := regexp.MustCompile(`[^a-z0-9-]+`)
	name = re.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-")
	for strings.Contains(name, "--") {
		name = strings.ReplaceAll(name, "--", "-")
	}
	if len(name) > 63 {
		name = strings.Trim(name[:63], "-")
	}
	return name
}

func deployTrackingWorker(ctx context.Context, u *ui.UI, workerDir, workerName, dbName, trackingKey, adminKey string) (string, error) {
	if _, err := exec.LookPath("wrangler"); err != nil {
		return "", fmt.Errorf("wrangler not found in PATH")
	}

	workerDir = filepath.Clean(workerDir)
	if _, err := os.Stat(filepath.Join(workerDir, "wrangler.toml")); err != nil {
		return "", fmt.Errorf("worker dir missing wrangler.toml: %s", workerDir)
	}

	u.Err().Printf("deploy\tstarting (worker=%s, db=%s)", workerName, dbName)

	dbID, err := ensureD1Database(ctx, workerDir, dbName)
	if err != nil {
		return "", err
	}

	if err := runWranglerCommand(ctx, workerDir, nil, "d1", "execute", dbName, "--file", "schema.sql", "--remote"); err != nil {
		return "", err
	}

	if err := runWranglerCommand(ctx, workerDir, strings.NewReader(trackingKey+"\n"), "secret", "put", "TRACKING_KEY", "--name", workerName); err != nil {
		return "", err
	}

	if err := runWranglerCommand(ctx, workerDir, strings.NewReader(adminKey+"\n"), "secret", "put", "ADMIN_KEY", "--name", workerName); err != nil {
		return "", err
	}

	configPath, err := writeWranglerConfig(workerDir, workerName, dbName, dbID)
	if err != nil {
		return "", err
	}
	defer os.Remove(configPath)

	if err := runWranglerCommand(ctx, workerDir, nil, "deploy", "--config", configPath, "--name", workerName); err != nil {
		return "", err
	}

	u.Err().Printf("deploy\tok")

	return dbID, nil
}

func ensureD1Database(ctx context.Context, workerDir, dbName string) (string, error) {
	out, err := runWranglerCommandOutput(ctx, workerDir, nil, "d1", "create", dbName)
	if err != nil {
		outInfo, infoErr := runWranglerCommandOutput(ctx, workerDir, nil, "d1", "info", dbName)
		if infoErr != nil {
			return "", err
		}
		id := parseDatabaseID(outInfo)
		if id == "" {
			return "", fmt.Errorf("failed to parse database_id from wrangler d1 info output")
		}
		return id, nil
	}

	id := parseDatabaseID(out)
	if id == "" {
		return "", fmt.Errorf("failed to parse database_id from wrangler d1 create output")
	}
	return id, nil
}

func parseDatabaseID(out string) string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`database_id\\s*=\\s*\"([^\"]+)\"`),
		regexp.MustCompile(`database_id\\s*:\\s*\"?([a-zA-Z0-9-]+)\"?`),
		regexp.MustCompile(`Database ID:\\s*([a-zA-Z0-9-]+)`),
	}
	for _, re := range patterns {
		if match := re.FindStringSubmatch(out); len(match) > 1 {
			return match[1]
		}
	}
	return ""
}

func writeWranglerConfig(workerDir, workerName, dbName, dbID string) (string, error) {
	templatePath := filepath.Join(workerDir, "wrangler.toml")
	data, err := os.ReadFile(templatePath)
	if err != nil {
		return "", fmt.Errorf("read wrangler.toml: %w", err)
	}

	content := string(data)
	content = replaceTomlString(content, "name", workerName)
	content = replaceTomlString(content, "database_name", dbName)
	content = replaceTomlString(content, "database_id", dbID)

	tmpFile, err := os.CreateTemp("", "gog-wrangler-*.toml")
	if err != nil {
		return "", fmt.Errorf("create temp wrangler config: %w", err)
	}
	defer tmpFile.Close()

	if _, err := tmpFile.WriteString(content); err != nil {
		return "", fmt.Errorf("write temp wrangler config: %w", err)
	}

	return tmpFile.Name(), nil
}

func replaceTomlString(content, key, value string) string {
	re := regexp.MustCompile(fmt.Sprintf(`(?m)^%s\\s*=\\s*\".*\"\\s*$`, regexp.QuoteMeta(key)))
	return re.ReplaceAllString(content, fmt.Sprintf(`%s = \"%s\"`, key, value))
}

func runWranglerCommand(ctx context.Context, dir string, input io.Reader, args ...string) error {
	_, err := runWranglerCommandOutput(ctx, dir, input, args...)
	return err
}

func runWranglerCommandOutput(ctx context.Context, dir string, input io.Reader, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "wrangler", args...)
	cmd.Dir = dir
	cmd.Stdin = input
	cmd.Env = append(os.Environ(), "WRANGLER_SEND_METRICS=false")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("wrangler %s failed: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}
