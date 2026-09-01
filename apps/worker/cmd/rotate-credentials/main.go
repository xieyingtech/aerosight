package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"aerosight/worker/internal/credentials"
	_ "github.com/jackc/pgx/v5/stdlib"
)

const rotationLockName = "aerosight.credential-rotation"

type credentialSource struct {
	table        string
	resourceType string
	scoped       bool
}

var credentialSources = []credentialSource{
	{table: "device_adapters", resourceType: "device-adapter", scoped: true},
	{table: "algorithm_providers", resourceType: "algorithm-provider", scoped: true},
	{table: "ai_providers", resourceType: "ai-provider", scoped: false},
	{table: "connector_asset_access_refs", resourceType: "flighthub-asset-reference", scoped: true},
}

type storedCredential struct {
	source   credentialSource
	id       int64
	scopeID  *int64
	raw      []byte
	envelope credentials.Envelope
}

func main() {
	dryRun := flag.Bool("dry-run", false, "validate every credential without writing")
	newSecretStdin := flag.Bool("new-secret-stdin", false, "read the new AUTH_SECRET from standard input")
	flag.Parse()
	if flag.NArg() != 0 {
		fatal(errors.New("unexpected positional arguments; secrets must not be passed as arguments"))
	}
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	oldSecret := os.Getenv("AUTH_SECRET")
	if databaseURL == "" || len(oldSecret) < 16 {
		fatal(errors.New("DATABASE_URL and the current AUTH_SECRET (at least 16 characters) are required"))
	}
	newSecret := ""
	if !*dryRun {
		var err error
		newSecret, err = readNewSecret(*newSecretStdin)
		if err != nil {
			fatal(err)
		}
		if newSecret == oldSecret {
			fatal(errors.New("new AUTH_SECRET must differ from the current value"))
		}
	}
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		fatal(err)
	}
	defer database.Close()
	counts, fingerprint, err := rotate(context.Background(), database, oldSecret, newSecret, *dryRun)
	if err != nil {
		fatal(err)
	}
	if *dryRun {
		fmt.Printf("Credential dry-run succeeded: device_adapters=%d algorithm_providers=%d ai_providers=%d connector_asset_access_refs=%d\n",
			counts["device_adapters"], counts["algorithm_providers"], counts["ai_providers"], counts["connector_asset_access_refs"])
		return
	}
	fmt.Printf("Credential rotation succeeded: device_adapters=%d algorithm_providers=%d ai_providers=%d connector_asset_access_refs=%d key_fingerprint=%s\n",
		counts["device_adapters"], counts["algorithm_providers"], counts["ai_providers"], counts["connector_asset_access_refs"], fingerprint)
	fmt.Println("Update the deployment AUTH_SECRET to the new value and restart Web and worker before leaving maintenance mode.")
}

func readNewSecret(fromStdin bool) (string, error) {
	if fromStdin {
		line, err := bufio.NewReader(io.LimitReader(os.Stdin, 16*1024)).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}
		secret := strings.TrimRight(line, "\r\n")
		if len(secret) < 16 {
			return "", errors.New("new AUTH_SECRET must contain at least 16 characters")
		}
		return secret, nil
	}
	first, err := readHiddenLine("New AUTH_SECRET: ")
	if err != nil {
		return "", err
	}
	defer clear(first)
	second, err := readHiddenLine("Confirm new AUTH_SECRET: ")
	if err != nil {
		return "", err
	}
	defer clear(second)
	if string(first) != string(second) {
		return "", errors.New("new AUTH_SECRET confirmation does not match")
	}
	if len(first) < 16 {
		return "", errors.New("new AUTH_SECRET must contain at least 16 characters")
	}
	return string(first), nil
}

func readHiddenLine(prompt string) ([]byte, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, errors.New("interactive input requires a terminal; use --new-secret-stdin for automation")
	}
	defer tty.Close()
	setEcho := func(enabled bool) error {
		argument := "-echo"
		if enabled {
			argument = "echo"
		}
		command := exec.Command("stty", argument)
		command.Stdin = tty
		return command.Run()
	}
	if err := setEcho(false); err != nil {
		return nil, errors.New("unable to disable terminal echo; use --new-secret-stdin for automation")
	}
	defer setEcho(true)
	if _, err := fmt.Fprint(tty, prompt); err != nil {
		return nil, err
	}
	line, err := bufio.NewReader(io.LimitReader(tty, 16*1024)).ReadString('\n')
	_, _ = fmt.Fprintln(tty)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return []byte(strings.TrimRight(line, "\r\n")), nil
}

func rotate(
	ctx context.Context,
	database *sql.DB,
	oldSecret string,
	newSecret string,
	dryRun bool,
) (map[string]int, string, error) {
	tx, err := database.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, "", err
	}
	defer tx.Rollback()
	var locked bool
	if err := tx.QueryRowContext(ctx, "select pg_try_advisory_xact_lock(hashtext($1))", rotationLockName).Scan(&locked); err != nil {
		return nil, "", err
	}
	if !locked {
		return nil, "", errors.New("another credential rotation is already running")
	}
	if _, err := tx.ExecContext(ctx, "set local lock_timeout = '2s'"); err != nil {
		return nil, "", err
	}
	stored, counts, err := readCredentials(ctx, tx)
	if err != nil {
		return nil, "", err
	}
	for _, item := range stored {
		var scopeID any
		if item.scopeID != nil {
			scopeID = *item.scopeID
		}
		aad := credentials.AAD(item.source.resourceType, item.id, scopeID)
		if dryRun {
			if err := credentials.ValidateEnvelope(item.envelope, oldSecret, aad); err != nil {
				return nil, "", fmt.Errorf("validate %s %d: %w", item.source.table, item.id, err)
			}
			continue
		}
		rotated, err := credentials.ReencryptEnvelope(item.envelope, oldSecret, newSecret, aad)
		if err != nil {
			return nil, "", fmt.Errorf("rotate %s %d: %w", item.source.table, item.id, err)
		}
		encoded, err := json.Marshal(rotated)
		if err != nil {
			return nil, "", err
		}
		statement := fmt.Sprintf(`update %s set credential_envelope_json = $2::jsonb, updated_at = now()
			where id = $1 and credential_envelope_json = $3::jsonb`, item.source.table)
		result, err := tx.ExecContext(ctx, statement, item.id, encoded, item.raw)
		if err != nil {
			return nil, "", err
		}
		if affected, _ := result.RowsAffected(); affected != 1 {
			return nil, "", fmt.Errorf("credential changed concurrently for %s %d", item.source.table, item.id)
		}
	}
	if dryRun {
		return counts, "", nil
	}
	if err := tx.Commit(); err != nil {
		return nil, "", err
	}
	fingerprint, err := credentials.KeyFingerprint(newSecret)
	return counts, fingerprint, err
}

func readCredentials(ctx context.Context, tx *sql.Tx) ([]storedCredential, map[string]int, error) {
	counts := map[string]int{}
	for _, source := range credentialSources {
		counts[source.table] = 0
	}
	var stored []storedCredential
	for _, source := range credentialSources {
		scopeProjection := "null::bigint"
		if source.scoped {
			scopeProjection = "project_id::bigint"
		}
		statement := fmt.Sprintf(`select id, %s, credential_envelope_json
			from %s where credential_envelope_json is not null order by id for update`, scopeProjection, source.table)
		rows, err := tx.QueryContext(ctx, statement)
		if err != nil {
			return nil, nil, err
		}
		for rows.Next() {
			var item storedCredential
			item.source = source
			if err := rows.Scan(&item.id, &item.scopeID, &item.raw); err != nil {
				rows.Close()
				return nil, nil, err
			}
			item.envelope, err = credentials.ParseEnvelope(item.raw)
			if err != nil {
				rows.Close()
				return nil, nil, fmt.Errorf("parse %s %d: %w", source.table, item.id, err)
			}
			stored = append(stored, item)
			counts[source.table]++
		}
		if err := rows.Close(); err != nil {
			return nil, nil, err
		}
		if err := rows.Err(); err != nil {
			return nil, nil, err
		}
	}
	return stored, counts, nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "Credential rotation failed:", err)
	os.Exit(1)
}
