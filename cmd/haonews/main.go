package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"hao.news/internal/apphost"
	"hao.news/internal/builtin"
	"hao.news/internal/extensions"
	"hao.news/internal/haonews"
	"hao.news/internal/haonews/live"
	"hao.news/internal/host"
	"hao.news/internal/scaffold"
	"hao.news/internal/themes/directorytheme"
	"hao.news/internal/workspace"
)

type boolFlag interface {
	IsBoolFlag() bool
}

type optionalBoolFlag struct {
	set   bool
	value bool
}

func (f *optionalBoolFlag) String() string {
	if f == nil {
		return ""
	}
	if !f.set {
		return ""
	}
	if f.value {
		return "true"
	}
	return "false"
}

func (f *optionalBoolFlag) Set(value string) error {
	if f == nil {
		return errors.New("optional bool flag is nil")
	}
	if strings.TrimSpace(value) == "" {
		f.set = true
		f.value = true
		return nil
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return err
	}
	f.set = true
	f.value = parsed
	return nil
}

func (f *optionalBoolFlag) IsBoolFlag() bool {
	return true
}

func (f *optionalBoolFlag) IsSet() bool {
	return f != nil && f.set
}

func (f *optionalBoolFlag) Value() bool {
	return f != nil && f.value
}

const identityOfflineBackupNotice = "Sensitive signing material was saved to the identity file. Back it up offline and do not share this file."

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return usageError()
	}
	switch args[0] {
	case "identity":
		return runIdentity(args[1:])
	case "publish":
		return runPublish(args[1:])
	case "verify":
		return runVerify(args[1:])
	case "show":
		return runShow(args[1:])
	case "sync":
		return runSync(args[1:])
	case "credit":
		return runCredit(args[1:])
	case "live":
		return runLive(args[1:])
	case "serve":
		return runServe(args[1:])
	case "plugins":
		return runPlugins(args[1:])
	case "themes":
		return runThemes(args[1:])
	case "apps":
		return runApps(args[1:])
	case "create":
		return runCreate(args[1:])
	default:
		return usageError()
	}
}

func runPublish(args []string) error {
	normalizedArgs, deprecatedReplyMagnet, err := normalizeDeprecatedPublishArgs(args)
	if err != nil {
		return err
	}
	if deprecatedReplyMagnet {
		fmt.Fprintln(os.Stderr, "warning: --reply-magnet is deprecated; use --reply-infohash or --reply-ref")
	}
	fs := flag.NewFlagSet("publish", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	storeRoot := fs.String("store", ".haonews", "store root")
	author := fs.String("author", "", "agent author id")
	identityFile := fs.String("identity-file", "", "path to a signing identity JSON file")
	kind := fs.String("kind", "post", "message kind")
	channel := fs.String("channel", "", "message channel")
	title := fs.String("title", "", "message title")
	body := fs.String("body", "", "message body")
	replyInfoHash := fs.String("reply-infohash", "", "reply target infohash")
	replyRef := fs.String("reply-ref", "", "reply target sync ref")
	tagsCSV := fs.String("tags", "", "comma-separated tags")
	extensionsJSON := fs.String("extensions-json", "", "inline JSON object for message extensions")
	extensionsFile := fs.String("extensions-file", "", "path to JSON object file for message extensions")
	if err := fs.Parse(normalizedArgs); err != nil {
		return err
	}
	if strings.TrimSpace(*identityFile) == "" {
		return errors.New("identity-file is required; all new posts and replies must be signed")
	}

	store, err := haonews.OpenStore(*storeRoot)
	if err != nil {
		return err
	}
	identity, err := haonews.LoadAgentIdentity(strings.TrimSpace(*identityFile))
	if err != nil {
		return err
	}
	if strings.TrimSpace(*author) == "" && strings.TrimSpace(identity.Author) != "" {
		*author = strings.TrimSpace(identity.Author)
	}
	if strings.TrimSpace(*author) == "" {
		return errors.New("author is required; set --author or store author in identity-file")
	}
	if strings.TrimSpace(identity.Author) != "" &&
		strings.TrimSpace(*author) != strings.TrimSpace(identity.Author) &&
		!(identity.HDEnabled && strings.TrimSpace(identity.Mnemonic) != "") {
		return errors.New("author does not match identity-file author")
	}

	resolvedReplyInfoHash := strings.TrimSpace(*replyInfoHash)
	if resolvedReplyInfoHash == "" && strings.TrimSpace(*replyRef) != "" {
		ref, err := haonews.ParseSyncRef(strings.TrimSpace(*replyRef))
		if err != nil {
			return fmt.Errorf("parse reply-ref: %w", err)
		}
		resolvedReplyInfoHash = ref.InfoHash
	}
	var replyTo *haonews.MessageLink
	if resolvedReplyInfoHash != "" {
		replyTo = &haonews.MessageLink{
			InfoHash: resolvedReplyInfoHash,
		}
	}
	extensions, err := loadJSONObject(*extensionsJSON, *extensionsFile)
	if err != nil {
		return err
	}
	extensions = ensurePublishProject(extensions)

	result, err := haonews.PublishMessage(store, haonews.MessageInput{
		Kind:       *kind,
		Author:     *author,
		Channel:    *channel,
		Title:      *title,
		Body:       *body,
		ReplyTo:    replyTo,
		Tags:       splitCSV(*tagsCSV),
		Identity:   &identity,
		Extensions: extensions,
		CreatedAt:  time.Now().UTC(),
	})
	if err != nil {
		return err
	}
	return writeJSON(result)
}

func ensurePublishProject(extensions map[string]any) map[string]any {
	if extensions == nil {
		return map[string]any{"project": "hao.news"}
	}
	if value, ok := extensions["project"].(string); ok && strings.TrimSpace(value) != "" {
		return extensions
	}
	extensions["project"] = "hao.news"
	return extensions
}

func runIdentity(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: haonews identity <init|create-hd|derive|derive-credit|list|recover> [flags]")
	}
	switch args[0] {
	case "init":
		return runIdentityInit(args[1:])
	case "create-hd":
		return runIdentityCreateHD(args[1:])
	case "derive":
		return runIdentityDerive(args[1:])
	case "derive-credit":
		return runIdentityDeriveCredit(args[1:])
	case "list":
		return runIdentityList(args[1:])
	case "recover":
		return runIdentityRecover(args[1:])
	case "registry":
		return runIdentityRegistry(args[1:])
	default:
		return errors.New("usage: haonews identity <init|create-hd|derive|derive-credit|list|recover|registry> [flags]")
	}
}

func runCredit(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: haonews credit <balance|proofs|stats|derive-key|clean|archive> [flags]")
	}
	switch args[0] {
	case "balance":
		return runCreditBalance(args[1:])
	case "proofs":
		return runCreditProofs(args[1:])
	case "stats":
		return runCreditStats(args[1:])
	case "derive-key":
		return runCreditDeriveKey(args[1:])
	case "clean":
		return runCreditClean(args[1:])
	case "archive":
		return runCreditArchive(args[1:])
	default:
		return errors.New("usage: haonews credit <balance|proofs|stats|derive-key|clean|archive> [flags]")
	}
}

func runLive(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: haonews live <host|join|list|archive|task-update> [flags]")
	}
	switch args[0] {
	case "host":
		return runLiveHost(args[1:])
	case "join":
		return runLiveJoin(args[1:])
	case "list":
		return runLiveList(args[1:])
	case "archive":
		return runLiveArchive(args[1:])
	case "task-update":
		return runLiveTaskUpdate(args[1:])
	default:
		return errors.New("usage: haonews live <host|join|list|archive|task-update> [flags]")
	}
}

func runLiveHost(args []string) error {
	fs := flag.NewFlagSet("live host", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	storeRoot := fs.String("store", ".haonews", "store root")
	netPath := fs.String("net", "", "network bootstrap config path")
	identityFile := fs.String("identity-file", "", "path to a signing identity JSON file")
	author := fs.String("author", "", "author id override")
	roomID := fs.String("room-id", "", "room id override")
	title := fs.String("title", "", "live room title")
	channel := fs.String("channel", "hao.news/live", "archive channel hint")
	var autoArchive optionalBoolFlag
	fs.Var(&autoArchive, "archive-on-exit", "publish archive automatically when the host exits")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*identityFile) == "" {
		return errors.New("identity-file is required")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	info, err := live.Host(ctx, live.SessionOptions{
		StoreRoot:    *storeRoot,
		NetPath:      *netPath,
		IdentityFile: *identityFile,
		Author:       *author,
		RoomID:       *roomID,
		Title:        *title,
		Channel:      *channel,
		Role:         "host",
		AutoArchive:  resolveLiveHostAutoArchive(&autoArchive),
	}, os.Stdin, os.Stdout)
	if err != nil {
		return err
	}
	return writeJSON(info)
}

func runLiveJoin(args []string) error {
	fs := flag.NewFlagSet("live join", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	storeRoot := fs.String("store", ".haonews", "store root")
	netPath := fs.String("net", "", "network bootstrap config path")
	identityFile := fs.String("identity-file", "", "path to a signing identity JSON file")
	author := fs.String("author", "", "author id override")
	roomID := fs.String("room-id", "", "room id to join")
	title := fs.String("title", "", "local title override")
	channel := fs.String("channel", "hao.news/live", "archive channel hint")
	role := fs.String("role", "participant", "join role: participant or viewer")
	var autoArchive optionalBoolFlag
	fs.Var(&autoArchive, "archive-on-exit", "publish archive automatically when this node exits")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*identityFile) == "" {
		return errors.New("identity-file is required")
	}
	if strings.TrimSpace(*roomID) == "" {
		return errors.New("room-id is required")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	resolvedRole := normalizeLiveJoinRole(*role)
	info, err := live.Join(ctx, live.SessionOptions{
		StoreRoot:    *storeRoot,
		NetPath:      *netPath,
		IdentityFile: *identityFile,
		Author:       *author,
		RoomID:       *roomID,
		Title:        *title,
		Channel:      *channel,
		Role:         resolvedRole,
		AutoArchive:  resolveLiveJoinAutoArchive(resolvedRole, &autoArchive),
	}, os.Stdin, os.Stdout)
	if err != nil {
		return err
	}
	return writeJSON(info)
}

func normalizeLiveJoinRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "", "participant":
		return "participant"
	case "viewer":
		return "viewer"
	default:
		return "participant"
	}
}

func resolveLiveHostAutoArchive(flag *optionalBoolFlag) bool {
	if flag != nil && flag.IsSet() {
		return flag.Value()
	}
	return true
}

func resolveLiveJoinAutoArchive(role string, flag *optionalBoolFlag) bool {
	if flag != nil && flag.IsSet() {
		return flag.Value()
	}
	return strings.TrimSpace(role) != "viewer"
}

func runLiveList(args []string) error {
	fs := flag.NewFlagSet("live list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	storeRoot := fs.String("store", ".haonews", "store root")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rooms, err := live.List(*storeRoot)
	if err != nil {
		return err
	}
	if rooms == nil {
		rooms = []live.RoomSummary{}
	}
	return writeJSON(rooms)
}

func runLiveArchive(args []string) error {
	fs := flag.NewFlagSet("live archive", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	storeRoot := fs.String("store", ".haonews", "store root")
	identityFile := fs.String("identity-file", "", "path to a signing identity JSON file")
	author := fs.String("author", "", "author id override")
	roomID := fs.String("room-id", "", "room id to archive")
	channel := fs.String("channel", "hao.news/live", "archive channel")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*identityFile) == "" {
		return errors.New("identity-file is required")
	}
	if strings.TrimSpace(*roomID) == "" {
		return errors.New("room-id is required")
	}
	result, err := live.Archive(live.ArchiveOptions{
		StoreRoot:    *storeRoot,
		IdentityFile: *identityFile,
		Author:       *author,
		RoomID:       *roomID,
		Channel:      *channel,
	})
	if err != nil {
		return err
	}
	return writeJSON(result)
}

func runLiveTaskUpdate(args []string) error {
	fs := flag.NewFlagSet("live task-update", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	storeRoot := fs.String("store", ".haonews", "store root")
	netPath := fs.String("net", "", "network bootstrap config path")
	identityFile := fs.String("identity-file", "", "path to a signing identity JSON file")
	author := fs.String("author", "", "author id override")
	roomID := fs.String("room-id", "", "room id")
	taskID := fs.String("task-id", "", "task id")
	statusValue := fs.String("status", "", "task status")
	description := fs.String("description", "", "task description")
	assignedTo := fs.String("assigned-to", "", "comma-separated assignees")
	progress := fs.Int("progress", -1, "task progress percent")
	channel := fs.String("channel", "hao.news/live", "archive channel hint")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*identityFile) == "" {
		return errors.New("identity-file is required")
	}
	if strings.TrimSpace(*roomID) == "" {
		return errors.New("room-id is required")
	}
	metadata := map[string]any{}
	if strings.TrimSpace(*taskID) != "" {
		metadata["task_id"] = strings.TrimSpace(*taskID)
	}
	if strings.TrimSpace(*statusValue) != "" {
		metadata["status"] = strings.TrimSpace(*statusValue)
	}
	if strings.TrimSpace(*description) != "" {
		metadata["description"] = strings.TrimSpace(*description)
	}
	if values := splitCSV(*assignedTo); len(values) > 0 {
		metadata["assigned_to"] = values
	}
	if *progress >= 0 {
		metadata["progress"] = *progress
	}
	if len(metadata) == 0 {
		return errors.New("at least one task-update field is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	info, err := live.PublishTaskUpdate(ctx, live.SessionOptions{
		StoreRoot:    *storeRoot,
		NetPath:      *netPath,
		IdentityFile: *identityFile,
		Author:       *author,
		RoomID:       *roomID,
		Channel:      *channel,
	}, metadata)
	if err != nil {
		return err
	}
	return writeJSON(map[string]any{
		"room":     info,
		"metadata": metadata,
		"type":     live.TypeTaskUpdate,
	})
}

func runIdentityRegistry(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: haonews identity registry <add|list|remove> [flags]")
	}
	switch args[0] {
	case "add":
		return runIdentityRegistryAdd(args[1:])
	case "list":
		return runIdentityRegistryList(args[1:])
	case "remove":
		return runIdentityRegistryRemove(args[1:])
	default:
		return errors.New("usage: haonews identity registry <add|list|remove> [flags]")
	}
}

func runIdentityInit(args []string) error {
	fs := flag.NewFlagSet("identity init", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	agentID := fs.String("agent-id", "", "stable agent id")
	author := fs.String("author", "", "default author for this identity")
	out := fs.String("out", "", "identity file output path; defaults to ~/.hao-news/identities/<sanitized-agent-id>.json")
	force := fs.Bool("force", false, "overwrite output file if it exists")
	if err := fs.Parse(args); err != nil {
		return err
	}
	outputPath, err := defaultIdentityOutputPath(*agentID, *out)
	if err != nil {
		return err
	}
	if !*force {
		if _, err := os.Stat(outputPath); err == nil {
			return fmt.Errorf("identity file already exists: %s", outputPath)
		}
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	identity, err := haonews.NewAgentIdentity(*agentID, *author, time.Now().UTC())
	if err != nil {
		return err
	}
	if err := haonews.SaveAgentIdentity(outputPath, identity); err != nil {
		return err
	}
	return writeJSON(addIdentityBackupNotice(map[string]any{
		"agent_id":   identity.AgentID,
		"author":     identity.Author,
		"key_type":   identity.KeyType,
		"public_key": identity.PublicKey,
		"created_at": identity.CreatedAt,
		"file":       outputPath,
	}, outputPath))
}

func runIdentityCreateHD(args []string) error {
	fs := flag.NewFlagSet("identity create-hd", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	agentID := fs.String("agent-id", "", "stable agent id")
	author := fs.String("author", "", "root author for this HD identity")
	out := fs.String("out", "", "identity file output path; defaults to ~/.hao-news/identities/<sanitized-author>.json")
	force := fs.Bool("force", false, "overwrite output file if it exists")
	if err := fs.Parse(args); err != nil {
		return err
	}
	outputPath, err := defaultIdentityOutputPath(*author, *out)
	if err != nil {
		return err
	}
	if !*force {
		if _, err := os.Stat(outputPath); err == nil {
			return fmt.Errorf("identity file already exists: %s", outputPath)
		}
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	identity, err := haonews.NewHDMasterIdentity(*agentID, *author, "", time.Now().UTC())
	if err != nil {
		return err
	}
	if err := haonews.SaveAgentIdentity(outputPath, identity); err != nil {
		return err
	}
	return writeJSON(identitySummaryForSavedIdentity(identity, outputPath))
}

func runIdentityRecover(args []string) error {
	fs := flag.NewFlagSet("identity recover", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	agentID := fs.String("agent-id", "", "stable agent id")
	author := fs.String("author", "", "root author for this HD identity")
	mnemonic := fs.String("mnemonic", "", "deprecated insecure input; use --mnemonic-file or --mnemonic-stdin")
	mnemonicFile := fs.String("mnemonic-file", "", "path to a file that contains the BIP39 mnemonic")
	mnemonicStdin := fs.Bool("mnemonic-stdin", false, "read the BIP39 mnemonic from stdin")
	out := fs.String("out", "", "identity file output path; defaults to ~/.hao-news/identities/<sanitized-author>.json")
	force := fs.Bool("force", false, "overwrite output file if it exists")
	if err := fs.Parse(args); err != nil {
		return err
	}
	mnemonicValue, err := resolveRecoveryMnemonic(*mnemonic, *mnemonicFile, *mnemonicStdin, os.Stdin)
	if err != nil {
		return err
	}
	outputPath, err := defaultIdentityOutputPath(*author, *out)
	if err != nil {
		return err
	}
	if !*force {
		if _, err := os.Stat(outputPath); err == nil {
			return fmt.Errorf("identity file already exists: %s", outputPath)
		}
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	identity, err := haonews.RecoverHDIdentity(*agentID, *author, mnemonicValue, time.Now().UTC())
	if err != nil {
		return err
	}
	if err := haonews.SaveAgentIdentity(outputPath, identity); err != nil {
		return err
	}
	return writeJSON(identitySummaryForSavedIdentity(identity, outputPath))
}

func runIdentityDerive(args []string) error {
	fs := flag.NewFlagSet("identity derive", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	identityFile := fs.String("identity-file", "", "path to the HD master identity JSON file")
	author := fs.String("author", "", "child author to derive, for example agent://alice/work")
	out := fs.String("out", "", "child signing identity output path")
	force := fs.Bool("force", false, "overwrite output file if it exists")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*identityFile) == "" {
		return errors.New("identity-file is required")
	}
	parentIdentity, err := haonews.LoadAgentIdentity(strings.TrimSpace(*identityFile))
	if err != nil {
		return err
	}
	childIdentity, err := haonews.DeriveChildIdentity(parentIdentity, *author, time.Now().UTC())
	if err != nil {
		return err
	}
	outputPath, err := defaultIdentityOutputPath(childIdentity.Author, *out)
	if err != nil {
		return err
	}
	if !*force {
		if _, err := os.Stat(outputPath); err == nil {
			return fmt.Errorf("identity file already exists: %s", outputPath)
		}
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	if err := haonews.SaveAgentIdentity(outputPath, childIdentity); err != nil {
		return err
	}
	return writeJSON(identitySummary(childIdentity, outputPath))
}

func runIdentityDeriveCredit(args []string) error {
	return runCreditDeriveKey(args)
}

func runIdentityList(args []string) error {
	fs := flag.NewFlagSet("identity list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dir := fs.String("dir", "", "identity directory; defaults to ~/.hao-news/identities")
	parent := fs.String("parent", "", "optional root or parent author filter")
	if err := fs.Parse(args); err != nil {
		return err
	}
	identityDir, err := defaultIdentityDir(*dir)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(identityDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return writeJSON([]map[string]any{})
		}
		return err
	}
	items := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(identityDir, entry.Name())
		identity, err := haonews.LoadAgentIdentity(path)
		if err != nil {
			continue
		}
		if !identityMatchesParentFilter(identity, *parent) {
			continue
		}
		items = append(items, identitySummary(identity, path))
	}
	return writeJSON(items)
}

func runCreditBalance(args []string) error {
	fs := flag.NewFlagSet("credit balance", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	author := fs.String("author", "", "author URI to query")
	storeRoot := fs.String("store", ".hao-news", "credit store root directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	store, err := haonews.OpenCreditStore(*storeRoot)
	if err != nil {
		return err
	}
	if strings.TrimSpace(*author) != "" {
		return writeJSON(store.GetBalance(*author))
	}
	balances, err := store.GetAllBalances()
	if err != nil {
		return err
	}
	return writeJSON(balances)
}

func runCreditProofs(args []string) error {
	fs := flag.NewFlagSet("credit proofs", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	date := fs.String("date", "", "date to query (YYYY-MM-DD)")
	author := fs.String("author", "", "author URI to query")
	start := fs.String("start", "", "start date (YYYY-MM-DD)")
	end := fs.String("end", "", "end date (YYYY-MM-DD)")
	storeRoot := fs.String("store", ".hao-news", "credit store root directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	store, err := haonews.OpenCreditStore(*storeRoot)
	if err != nil {
		return err
	}
	switch {
	case strings.TrimSpace(*date) != "":
		proofs, err := store.GetProofsByDate(*date)
		if err != nil {
			return err
		}
		return writeJSON(proofs)
	case strings.TrimSpace(*author) != "":
		proofs, err := store.GetProofsByAuthor(*author, *start, *end)
		if err != nil {
			return err
		}
		return writeJSON(proofs)
	default:
		proofs, err := store.GetProofsByDate(time.Now().UTC().Format("2006-01-02"))
		if err != nil {
			return err
		}
		return writeJSON(proofs)
	}
}

func runCreditStats(args []string) error {
	fs := flag.NewFlagSet("credit stats", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	storeRoot := fs.String("store", ".hao-news", "credit store root directory")
	dailyLimit := fs.Int("daily-limit", 7, "number of recent daily stats to include")
	if err := fs.Parse(args); err != nil {
		return err
	}
	store, err := haonews.OpenCreditStore(*storeRoot)
	if err != nil {
		return err
	}
	balances, err := store.GetAllBalances()
	if err != nil {
		return err
	}
	totalCredits := 0
	for _, balance := range balances {
		totalCredits += balance.Credits
	}
	dailyStats, err := store.GetDailyStats(*dailyLimit)
	if err != nil {
		return err
	}
	witnessRoles, err := store.GetWitnessRoleStats()
	if err != nil {
		return err
	}
	return writeJSON(map[string]any{
		"total_nodes":   len(balances),
		"total_credits": totalCredits,
		"suspicious":    store.ValidateBalanceIntegrity(),
		"balances":      balances,
		"daily":         dailyStats,
		"witness_roles": witnessRoles,
		"daily_limit":   *dailyLimit,
	})
}

func runCreditDeriveKey(args []string) error {
	fs := flag.NewFlagSet("credit derive-key", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	parentFile := fs.String("parent", "", "path to the HD master identity JSON file")
	identityFile := fs.String("identity-file", "", "alias for --parent")
	out := fs.String("out", "", "credit identity output path")
	force := fs.Bool("force", false, "overwrite output file if it exists")
	if err := fs.Parse(args); err != nil {
		return err
	}
	input := strings.TrimSpace(*parentFile)
	if input == "" {
		input = strings.TrimSpace(*identityFile)
	}
	if input == "" {
		return errors.New("parent is required")
	}
	parentIdentity, err := haonews.LoadAgentIdentity(input)
	if err != nil {
		return err
	}
	creditIdentity, err := haonews.DeriveCreditOnlineKey(parentIdentity)
	if err != nil {
		return err
	}
	outputPath, err := defaultIdentityOutputPath(creditIdentity.Author, *out)
	if err != nil {
		return err
	}
	if !*force {
		if _, err := os.Stat(outputPath); err == nil {
			return fmt.Errorf("identity file already exists: %s", outputPath)
		}
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	if err := haonews.SaveAgentIdentity(outputPath, creditIdentity); err != nil {
		return err
	}
	return writeJSON(identitySummaryForSavedIdentity(creditIdentity, outputPath))
}

func runCreditClean(args []string) error {
	fs := flag.NewFlagSet("credit clean", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	keepDays := fs.Int("keep-days", 90, "keep proofs for this many days")
	storeRoot := fs.String("store", ".hao-news", "credit store root directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	store, err := haonews.OpenCreditStore(*storeRoot)
	if err != nil {
		return err
	}
	removed, err := store.CleanOldProofs(*keepDays)
	if err != nil {
		return err
	}
	return writeJSON(map[string]any{
		"removed":   removed,
		"keep_days": *keepDays,
		"store":     store.Root,
	})
}

func runCreditArchive(args []string) error {
	fs := flag.NewFlagSet("credit archive", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	keepDays := fs.Int("keep-days", 90, "keep live proofs for this many days before archiving older days")
	storeRoot := fs.String("store", ".hao-news", "credit store root directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	store, err := haonews.OpenCreditStore(*storeRoot)
	if err != nil {
		return err
	}
	archivedDays, archivedProofs, err := store.ArchiveProofs(*keepDays)
	if err != nil {
		return err
	}
	return writeJSON(map[string]any{
		"archived_days":   archivedDays,
		"archived_proofs": archivedProofs,
		"keep_days":       *keepDays,
		"store":           store.Root,
	})
}

func runIdentityRegistryAdd(args []string) error {
	fs := flag.NewFlagSet("identity registry add", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	registryPath := fs.String("registry", "", "registry file path; defaults to ~/.hao-news/identity_registry.json")
	author := fs.String("author", "", "root author to register")
	pubkey := fs.String("pubkey", "", "master public key")
	trustLevel := fs.String("trust-level", "known", "trust level: trusted, known, unknown")
	notes := fs.String("notes", "", "optional notes")
	if err := fs.Parse(args); err != nil {
		return err
	}
	path, err := defaultIdentityRegistryPath(*registryPath)
	if err != nil {
		return err
	}
	registry, err := haonews.LoadIdentityRegistry(path)
	if err != nil {
		return err
	}
	if err := registry.Add(*author, *pubkey, *trustLevel, *notes, time.Now().UTC()); err != nil {
		return err
	}
	if err := registry.Save(path); err != nil {
		return err
	}
	entry, _ := registry.Get(*author)
	return writeJSON(map[string]any{
		"author":        *author,
		"master_pubkey": entry.MasterPubKey,
		"trust_level":   entry.TrustLevel,
		"added_at":      entry.AddedAt,
		"notes":         entry.Notes,
		"registry_file": path,
	})
}

func runIdentityRegistryList(args []string) error {
	fs := flag.NewFlagSet("identity registry list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	registryPath := fs.String("registry", "", "registry file path; defaults to ~/.hao-news/identity_registry.json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	path, err := defaultIdentityRegistryPath(*registryPath)
	if err != nil {
		return err
	}
	registry, err := haonews.LoadIdentityRegistry(path)
	if err != nil {
		return err
	}
	items := make([]map[string]any, 0, len(registry.Entries))
	for author, entry := range registry.Entries {
		items = append(items, map[string]any{
			"author":        author,
			"master_pubkey": entry.MasterPubKey,
			"trust_level":   entry.TrustLevel,
			"added_at":      entry.AddedAt,
			"notes":         entry.Notes,
		})
	}
	return writeJSON(map[string]any{
		"registry_file": path,
		"entries":       items,
	})
}

func runIdentityRegistryRemove(args []string) error {
	fs := flag.NewFlagSet("identity registry remove", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	registryPath := fs.String("registry", "", "registry file path; defaults to ~/.hao-news/identity_registry.json")
	author := fs.String("author", "", "root author to remove")
	if err := fs.Parse(args); err != nil {
		return err
	}
	path, err := defaultIdentityRegistryPath(*registryPath)
	if err != nil {
		return err
	}
	registry, err := haonews.LoadIdentityRegistry(path)
	if err != nil {
		return err
	}
	removed := registry.Remove(*author)
	if err := registry.Save(path); err != nil {
		return err
	}
	return writeJSON(map[string]any{
		"author":        *author,
		"removed":       removed,
		"registry_file": path,
	})
}

func runVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dir := fs.String("dir", "", "content directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*dir) == "" {
		return errors.New("dir is required")
	}
	msg, body, err := haonews.LoadMessage(*dir)
	if err != nil {
		return err
	}
	return writeJSON(struct {
		Valid   bool            `json:"valid"`
		Message haonews.Message `json:"message"`
		BodyLen int             `json:"body_len"`
	}{
		Valid:   true,
		Message: msg,
		BodyLen: len(body),
	})
}

func runShow(args []string) error {
	fs := flag.NewFlagSet("show", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dir := fs.String("dir", "", "content directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*dir) == "" {
		return errors.New("dir is required")
	}
	msg, body, err := haonews.LoadMessage(*dir)
	if err != nil {
		return err
	}
	return writeJSON(struct {
		Message haonews.Message `json:"message"`
		Body    string          `json:"body"`
	}{
		Message: msg,
		Body:    body,
	})
}

func runSync(args []string) error {
	normalizedArgs, deprecatedMagnetFlag, err := normalizeDeprecatedSyncArgs(args)
	if err != nil {
		return err
	}
	if deprecatedMagnetFlag {
		fmt.Fprintln(os.Stderr, "warning: --magnet is deprecated; use --ref")
	}
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	storeRoot := fs.String("store", ".haonews", "store root")
	queuePath := fs.String("queue", "", "line-based sync ref/infohash queue file")
	netPath := fs.String("net", "./haonews_net.inf", "network bootstrap config")
	subscriptionsPath := fs.String("subscriptions", "", "subscription rules file for pubsub topic joins")
	writerPolicyPath := fs.String("writer-policy", "", "writer policy file reserved for sync validation and filtering")
	creditIdentityFile := fs.String("credit-identity-file", "", "path to credit identity JSON file for auto proof generation")
	refsCSV := fs.String("ref", "", "comma-separated sync refs or infohashes to sync immediately")
	poll := fs.Duration("poll", 30*time.Second, "queue polling interval")
	timeout := fs.Duration("timeout", 20*time.Second, "per-ref sync timeout")
	once := fs.Bool("once", false, "run one sync pass and exit")
	seed := fs.Bool("seed", true, "seed after download while daemon is running")
	directTransfer := fs.Bool("direct-transfer", true, "prefer libp2p direct bundle transfer before HTTP fallback")
	if err := fs.Parse(normalizedArgs); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	_ = *writerPolicyPath

	return haonews.RunSync(ctx, haonews.SyncOptions{
		StoreRoot:          *storeRoot,
		QueuePath:          *queuePath,
		NetPath:            *netPath,
		SubscriptionsPath:  *subscriptionsPath,
		CreditIdentityFile: *creditIdentityFile,
		Refs:               splitCSV(*refsCSV),
		PollInterval:       *poll,
		Timeout:            *timeout,
		Once:               *once,
		Seed:               *seed,
		DirectTransfer:     *directTransfer,
	}, nil)
}

func normalizeDeprecatedPublishArgs(args []string) ([]string, bool, error) {
	out := make([]string, 0, len(args))
	rewrote := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--reply-magnet":
			if i+1 >= len(args) {
				return nil, false, errors.New("--reply-magnet requires a value")
			}
			infoHash, err := resolveInfoHashFromLegacyRef(args[i+1])
			if err != nil {
				return nil, false, fmt.Errorf("parse --reply-magnet: %w", err)
			}
			out = append(out, "--reply-infohash", infoHash)
			i++
			rewrote = true
		case strings.HasPrefix(arg, "--reply-magnet="):
			infoHash, err := resolveInfoHashFromLegacyRef(strings.TrimPrefix(arg, "--reply-magnet="))
			if err != nil {
				return nil, false, fmt.Errorf("parse --reply-magnet: %w", err)
			}
			out = append(out, "--reply-infohash="+infoHash)
			rewrote = true
		default:
			out = append(out, arg)
		}
	}
	return out, rewrote, nil
}

func normalizeDeprecatedSyncArgs(args []string) ([]string, bool, error) {
	out := make([]string, 0, len(args))
	rewrote := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--magnet":
			if i+1 >= len(args) {
				return nil, false, errors.New("--magnet requires a value")
			}
			out = append(out, "--ref", args[i+1])
			i++
			rewrote = true
		case strings.HasPrefix(arg, "--magnet="):
			out = append(out, "--ref="+strings.TrimPrefix(arg, "--magnet="))
			rewrote = true
		default:
			out = append(out, arg)
		}
	}
	return out, rewrote, nil
}

func resolveInfoHashFromLegacyRef(raw string) (string, error) {
	ref, err := haonews.ParseSyncRef(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(ref.InfoHash) == "" {
		return "", errors.New("missing infohash")
	}
	return strings.TrimSpace(ref.InfoHash), nil
}

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	listenAddr := fs.String("listen", "0.0.0.0:51818", "http listen address")
	appID := fs.String("app", "", "built-in application id; defaults to the built-in sample app")
	appDir := fs.String("app-dir", "", "application directory containing haonews.app.json and optional themes/plugins folders")
	extensionsRoot := fs.String("extensions-root", "", "installed extensions root; defaults to ~/.haonews/extensions")
	pluginID := fs.String("plugin", "", "single built-in plugin id; ignored when --plugins is set")
	pluginsCSV := fs.String("plugins", "", "comma-separated built-in plugin ids to compose; overrides --plugin")
	pluginDirsCSV := fs.String("plugin-dir", "", "comma-separated external plugin directories containing haonews.plugin.json")
	themeID := fs.String("theme", "", "theme id; defaults to the plugin default theme")
	themeDir := fs.String("theme-dir", "", "directory theme override; expects haonews.theme.json plus templates/static")
	project := fs.String("project", "", "project id override")
	version := fs.String("version", "dev", "host version label")
	runtimeRoot := fs.String("runtime-root", "", "application runtime root")
	storeRoot := fs.String("store", "", "store root override")
	archiveRoot := fs.String("archive", "", "archive root override")
	rulesPath := fs.String("subscriptions", "", "subscription rules path override")
	writerPolicy := fs.String("writer-policy", "", "writer policy path override")
	netPath := fs.String("net", "", "network bootstrap config override")
	syncMode := fs.String("sync-mode", "", "sync mode override")
	syncBinary := fs.String("sync-binary", "", "managed sync binary override")
	syncStaleAfter := fs.Duration("sync-stale-after", 2*time.Minute, "managed sync stale restart threshold")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	instance, err := host.New(ctx, host.Config{
		App:              *appID,
		AppDir:           *appDir,
		ExtensionsRoot:   *extensionsRoot,
		Plugin:           *pluginID,
		Plugins:          splitCSV(*pluginsCSV),
		PluginDirs:       splitCSV(*pluginDirsCSV),
		Theme:            *themeID,
		ThemeDir:         *themeDir,
		Project:          *project,
		Version:          *version,
		ListenAddr:       *listenAddr,
		RuntimeRoot:      *runtimeRoot,
		StoreRoot:        *storeRoot,
		ArchiveRoot:      *archiveRoot,
		RulesPath:        *rulesPath,
		WriterPolicyPath: *writerPolicy,
		NetPath:          *netPath,
		SyncMode:         *syncMode,
		SyncBinaryPath:   *syncBinary,
		SyncStaleAfter:   *syncStaleAfter,
	})
	if err != nil {
		return err
	}
	return instance.ListenAndServe(ctx)
}

func runPlugins(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: haonews plugins <list|inspect|install|link|remove>")
	}
	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("plugins list", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		root := fs.String("root", "", "extensions root override")
		if err := parseFlagSetInterspersed(fs, args[1:]); err != nil {
			return err
		}
		registry := builtin.DefaultRegistry()
		store, err := extensions.Open(*root)
		if err != nil {
			return err
		}
		installed, err := store.ListPlugins()
		if err != nil {
			return err
		}
		plugins := make([]any, 0, len(registry.PluginManifests())+len(installed))
		for _, manifest := range registry.PluginManifests() {
			plugins = append(plugins, map[string]any{
				"source":   "builtin",
				"manifest": manifest,
			})
		}
		for _, entry := range installed {
			plugins = append(plugins, map[string]any{
				"source":   "installed",
				"root":     entry.Root,
				"manifest": entry.Manifest,
				"config":   entry.Config,
				"metadata": entry.Metadata,
			})
		}
		return writeJSON(struct {
			Plugins []any `json:"plugins"`
		}{
			Plugins: plugins,
		})
	case "inspect":
		fs := flag.NewFlagSet("plugins inspect", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		dir := fs.String("dir", "", "plugin directory containing haonews.plugin.json")
		root := fs.String("root", "", "extensions root override")
		if err := parseFlagSetInterspersed(fs, args[1:]); err != nil {
			return err
		}
		registry := builtin.DefaultRegistry()
		store, err := extensions.Open(*root)
		if err != nil {
			return err
		}
		if _, err := store.RegisterIntoRegistry(registry, "", "", ""); err != nil {
			return err
		}
		if strings.TrimSpace(*dir) != "" {
			bundle, err := workspace.LoadPluginBundleDir(*dir)
			if err != nil {
				return err
			}
			_, manifest, err := workspace.LoadPluginDir(*dir, registry)
			if err != nil {
				return err
			}
			resolved, err := workspace.ValidatePluginManifest(manifest, registry)
			if err != nil {
				return err
			}
			resolved.Root = bundle.Root
			resolved.Config = bundle.Config
			return writeJSON(struct {
				Dir      string                   `json:"dir"`
				Manifest apphost.PluginManifest   `json:"manifest"`
				Config   map[string]any           `json:"config,omitempty"`
				Resolved workspace.ResolvedPlugin `json:"resolved"`
			}{
				Dir:      *dir,
				Manifest: manifest,
				Config:   bundle.Config,
				Resolved: resolved,
			})
		}
		if fs.NArg() == 0 {
			return errors.New("plugin id or --dir is required")
		}
		id := fs.Arg(0)
		if entry, err := store.GetPlugin(id); err == nil {
			resolved, err := workspace.ValidatePluginManifest(entry.Manifest, registry)
			if err != nil {
				return err
			}
			resolved.Root = entry.Root
			resolved.Config = entry.Config
			return writeJSON(struct {
				Source   string                     `json:"source"`
				Root     string                     `json:"root"`
				Manifest apphost.PluginManifest     `json:"manifest"`
				Config   map[string]any             `json:"config,omitempty"`
				Metadata extensions.InstallMetadata `json:"metadata"`
				Resolved workspace.ResolvedPlugin   `json:"resolved"`
			}{
				Source:   "installed",
				Root:     entry.Root,
				Manifest: entry.Manifest,
				Config:   entry.Config,
				Metadata: entry.Metadata,
				Resolved: resolved,
			})
		}
		_, manifest, err := registry.ResolvePlugin(id)
		if err != nil {
			return err
		}
		return writeJSON(struct {
			Source   string                 `json:"source"`
			Manifest apphost.PluginManifest `json:"manifest"`
		}{
			Source:   "builtin",
			Manifest: manifest,
		})
	case "install", "link":
		fs := flag.NewFlagSet("plugins "+args[0], flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		dir := fs.String("dir", "", "plugin directory containing haonews.plugin.json")
		root := fs.String("root", "", "extensions root override")
		if err := parseFlagSetInterspersed(fs, args[1:]); err != nil {
			return err
		}
		store, err := extensions.Open(*root)
		if err != nil {
			return err
		}
		entry, err := store.InstallPlugin(*dir, args[0] == "link")
		if err != nil {
			return err
		}
		return writeJSON(entry)
	case "remove":
		fs := flag.NewFlagSet("plugins remove", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		root := fs.String("root", "", "extensions root override")
		if err := parseFlagSetInterspersed(fs, args[1:]); err != nil {
			return err
		}
		if fs.NArg() == 0 {
			return errors.New("plugin id is required")
		}
		store, err := extensions.Open(*root)
		if err != nil {
			return err
		}
		if err := store.RemovePlugin(fs.Arg(0)); err != nil {
			return err
		}
		return writeJSON(map[string]any{"removed": fs.Arg(0)})
	default:
		return errors.New("usage: haonews plugins <list|inspect|install|link|remove>")
	}
}

func runApps(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: haonews apps <list|inspect|validate|install|link|remove>")
	}
	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("apps list", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		root := fs.String("root", "", "extensions root override")
		if err := parseFlagSetInterspersed(fs, args[1:]); err != nil {
			return err
		}
		store, err := extensions.Open(*root)
		if err != nil {
			return err
		}
		installed, err := store.ListApps()
		if err != nil {
			return err
		}
		apps := make([]any, 0, len(builtin.DefaultApps())+len(installed))
		for _, app := range builtin.DefaultApps() {
			apps = append(apps, map[string]any{
				"source": "builtin",
				"app":    app,
			})
		}
		for _, entry := range installed {
			apps = append(apps, map[string]any{
				"source":   "installed",
				"root":     entry.Root,
				"app":      entry.Manifest,
				"config":   entry.Config,
				"metadata": entry.Metadata,
			})
		}
		return writeJSON(struct {
			Apps []any `json:"apps"`
		}{
			Apps: apps,
		})
	case "inspect":
		fs := flag.NewFlagSet("apps inspect", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		dir := fs.String("dir", "", "application directory containing haonews.app.json")
		root := fs.String("root", "", "extensions root override")
		if err := parseFlagSetInterspersed(fs, args[1:]); err != nil {
			return err
		}
		if strings.TrimSpace(*dir) != "" {
			bundle, report, err := inspectAppDir(*dir, *root)
			if err != nil {
				return err
			}
			return writeJSON(struct {
				Dir        string                     `json:"dir"`
				App        apphost.AppManifest        `json:"app"`
				Config     workspace.AppConfig        `json:"config"`
				Plugins    []apphost.PluginManifest   `json:"plugins"`
				Themes     []apphost.ThemeManifest    `json:"themes"`
				Validation workspace.ValidationReport `json:"validation"`
			}{
				Dir:        *dir,
				App:        bundle.App,
				Config:     bundle.Config,
				Plugins:    bundle.PluginManifests,
				Themes:     bundle.ThemeManifests,
				Validation: report,
			})
		}
		if fs.NArg() == 0 {
			return errors.New("app id or --dir is required")
		}
		store, err := extensions.Open(*root)
		if err != nil {
			return err
		}
		entry, err := store.GetApp(fs.Arg(0))
		if err != nil {
			return err
		}
		bundle, report, err := inspectAppDir(entry.Root, *root)
		if err != nil {
			return err
		}
		return writeJSON(struct {
			Source     string                     `json:"source"`
			Root       string                     `json:"root"`
			Metadata   extensions.InstallMetadata `json:"metadata"`
			App        apphost.AppManifest        `json:"app"`
			Config     workspace.AppConfig        `json:"config"`
			Plugins    []apphost.PluginManifest   `json:"plugins"`
			Themes     []apphost.ThemeManifest    `json:"themes"`
			Validation workspace.ValidationReport `json:"validation"`
		}{
			Source:     "installed",
			Root:       entry.Root,
			Metadata:   entry.Metadata,
			App:        bundle.App,
			Config:     bundle.Config,
			Plugins:    bundle.PluginManifests,
			Themes:     bundle.ThemeManifests,
			Validation: report,
		})
	case "validate":
		fs := flag.NewFlagSet("apps validate", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		dir := fs.String("dir", "", "application directory containing haonews.app.json")
		root := fs.String("root", "", "extensions root override")
		if err := parseFlagSetInterspersed(fs, args[1:]); err != nil {
			return err
		}
		target := strings.TrimSpace(*dir)
		if target == "" {
			if fs.NArg() == 0 {
				return errors.New("app id or --dir is required")
			}
			store, err := extensions.Open(*root)
			if err != nil {
				return err
			}
			entry, err := store.GetApp(fs.Arg(0))
			if err != nil {
				return err
			}
			target = entry.Root
		}
		_, report, err := inspectAppDir(target, *root)
		if err != nil {
			return err
		}
		return writeJSON(report)
	case "install", "link":
		fs := flag.NewFlagSet("apps "+args[0], flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		dir := fs.String("dir", "", "application directory containing haonews.app.json")
		root := fs.String("root", "", "extensions root override")
		if err := parseFlagSetInterspersed(fs, args[1:]); err != nil {
			return err
		}
		store, err := extensions.Open(*root)
		if err != nil {
			return err
		}
		entry, err := store.InstallApp(*dir, args[0] == "link")
		if err != nil {
			return err
		}
		return writeJSON(entry)
	case "remove":
		fs := flag.NewFlagSet("apps remove", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		root := fs.String("root", "", "extensions root override")
		if err := parseFlagSetInterspersed(fs, args[1:]); err != nil {
			return err
		}
		if fs.NArg() == 0 {
			return errors.New("app id is required")
		}
		store, err := extensions.Open(*root)
		if err != nil {
			return err
		}
		if err := store.RemoveApp(fs.Arg(0)); err != nil {
			return err
		}
		return writeJSON(map[string]any{"removed": fs.Arg(0)})
	default:
		return errors.New("usage: haonews apps <list|inspect|validate|install|link|remove>")
	}
}

func runCreate(args []string) error {
	if len(args) < 2 {
		return errors.New("usage: haonews create <plugin|theme|app> <name> [--out dir]")
	}
	kind := strings.TrimSpace(args[0])
	target := strings.TrimSpace(args[1])
	if target == "" {
		return errors.New("name is required")
	}
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	outDir := fs.String("out", "", "output directory")
	if err := fs.Parse(args[2:]); err != nil {
		return err
	}
	name, resolvedOut, err := resolveCreateTarget(target, *outDir)
	if err != nil {
		return err
	}

	var (
		files []scaffold.File
	)
	switch kind {
	case "plugin":
		files, err = scaffold.PluginFiles(name)
	case "theme":
		files, err = scaffold.ThemeFiles(name)
	case "app":
		files, err = scaffold.AppFiles(name)
	default:
		return errors.New("usage: haonews create <plugin|theme|app> <name> [--out dir]")
	}
	if err != nil {
		return err
	}
	if err := scaffold.WriteFiles(resolvedOut, files); err != nil {
		return err
	}
	return writeJSON(map[string]any{
		"kind":   kind,
		"name":   name,
		"output": resolvedOut,
		"files":  filePaths(files),
	})
}

func resolveCreateTarget(target, explicitOut string) (string, string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", "", errors.New("name is required")
	}
	explicitOut = strings.TrimSpace(explicitOut)
	if explicitOut != "" {
		return targetBaseName(target), explicitOut, nil
	}
	if looksLikePath(target) {
		return targetBaseName(target), target, nil
	}
	return target, scaffold.Slug(target), nil
}

func looksLikePath(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if filepath.IsAbs(value) {
		return true
	}
	switch value {
	case ".", "..":
		return true
	}
	return strings.Contains(value, "/") || strings.Contains(value, `\`)
}

func targetBaseName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	base := filepath.Base(filepath.Clean(value))
	base = strings.TrimSpace(base)
	if base == "." || base == string(filepath.Separator) || base == "" {
		return value
	}
	return base
}

func defaultIdentityOutputPath(agentID, explicitOut string) (string, error) {
	explicitOut = strings.TrimSpace(explicitOut)
	if explicitOut != "" {
		return explicitOut, nil
	}
	identityDir, err := defaultIdentityDir("")
	if err != nil {
		return "", err
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return "", errors.New("agent-id is required")
	}
	return filepath.Join(identityDir, sanitizeAgentIDForFilename(agentID)+".json"), nil
}

func sanitizeAgentIDForFilename(agentID string) string {
	agentID = strings.ToLower(strings.TrimSpace(agentID))
	if agentID == "" {
		return "identity"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range agentID {
		isAlnum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlnum {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	value := strings.Trim(b.String(), "-")
	if value == "" {
		return "identity"
	}
	return value
}

func defaultIdentityDir(explicitDir string) (string, error) {
	explicitDir = strings.TrimSpace(explicitDir)
	if explicitDir != "" {
		return explicitDir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	home = strings.TrimSpace(home)
	if home == "" {
		return "", errors.New("user home directory is empty")
	}
	return filepath.Join(home, ".hao-news", "identities"), nil
}

func defaultIdentityRegistryPath(explicitPath string) (string, error) {
	explicitPath = strings.TrimSpace(explicitPath)
	if explicitPath != "" {
		return explicitPath, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	home = strings.TrimSpace(home)
	if home == "" {
		return "", errors.New("user home directory is empty")
	}
	return filepath.Join(home, ".hao-news", "identity_registry.json"), nil
}

func identitySummary(identity haonews.AgentIdentity, path string) map[string]any {
	return map[string]any{
		"agent_id":             identity.AgentID,
		"author":               identity.Author,
		"key_type":             identity.KeyType,
		"public_key":           identity.PublicKey,
		"created_at":           identity.CreatedAt,
		"file":                 path,
		"hd_enabled":           identity.HDEnabled,
		"master_public_key":    identity.MasterPubKey,
		"derivation_path":      identity.DerivationPath,
		"parent":               identity.Parent,
		"parent_public_key":    identity.ParentPublicKey,
		"has_signing_material": strings.TrimSpace(identity.PrivateKey) != "" || strings.TrimSpace(identity.Mnemonic) != "",
	}
}

func identitySummaryForSavedIdentity(identity haonews.AgentIdentity, path string) map[string]any {
	return addIdentityBackupNotice(identitySummary(identity, path), path)
}

func addIdentityBackupNotice(summary map[string]any, path string) map[string]any {
	summary["sensitive_material_file"] = path
	summary["backup_notice"] = identityOfflineBackupNotice
	return summary
}

func resolveRecoveryMnemonic(legacyMnemonic, mnemonicFile string, mnemonicStdin bool, stdin io.Reader) (string, error) {
	if strings.TrimSpace(legacyMnemonic) != "" {
		return "", errors.New("using --mnemonic is disabled because it can leak secrets via shell history; use --mnemonic-file or --mnemonic-stdin")
	}
	sources := 0
	if strings.TrimSpace(mnemonicFile) != "" {
		sources++
	}
	if mnemonicStdin {
		sources++
	}
	if sources == 0 {
		return "", errors.New("mnemonic input is required; use --mnemonic-file or --mnemonic-stdin")
	}
	if sources > 1 {
		return "", errors.New("use exactly one mnemonic source: --mnemonic-file or --mnemonic-stdin")
	}
	if strings.TrimSpace(mnemonicFile) != "" {
		data, err := os.ReadFile(strings.TrimSpace(mnemonicFile))
		if err != nil {
			return "", err
		}
		value := strings.TrimSpace(string(data))
		if value == "" {
			return "", errors.New("mnemonic file is empty")
		}
		return value, nil
	}
	if stdin == nil {
		return "", errors.New("stdin reader is not available")
	}
	data, err := io.ReadAll(stdin)
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(data))
	if value == "" {
		return "", errors.New("mnemonic stdin input is empty")
	}
	return value, nil
}

func identityMatchesParentFilter(identity haonews.AgentIdentity, parent string) bool {
	parent = strings.TrimSpace(parent)
	if parent == "" {
		return true
	}
	if identity.Author == parent || identity.Parent == parent {
		return true
	}
	return strings.HasPrefix(identity.Author, parent+"/")
}

func parseFlagSetInterspersed(fs *flag.FlagSet, args []string) error {
	reordered := make([]string, 0, len(args))
	positionals := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positionals = append(positionals, arg)
			continue
		}
		reordered = append(reordered, arg)
		if strings.Contains(arg, "=") {
			continue
		}
		name := strings.TrimLeft(arg, "-")
		if name == "" {
			continue
		}
		info := fs.Lookup(name)
		if info == nil {
			continue
		}
		if bf, ok := info.Value.(boolFlag); ok && bf.IsBoolFlag() {
			continue
		}
		if i+1 < len(args) {
			i++
			reordered = append(reordered, args[i])
		}
	}
	reordered = append(reordered, positionals...)
	return fs.Parse(reordered)
}

func filePaths(files []scaffold.File) []string {
	out := make([]string, 0, len(files))
	for _, file := range files {
		out = append(out, file.Path)
	}
	return out
}

func runThemes(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: haonews themes <list|inspect|install|link|remove>")
	}
	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("themes list", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		root := fs.String("root", "", "extensions root override")
		if err := parseFlagSetInterspersed(fs, args[1:]); err != nil {
			return err
		}
		registry := builtin.DefaultRegistry()
		store, err := extensions.Open(*root)
		if err != nil {
			return err
		}
		installed, err := store.ListThemes()
		if err != nil {
			return err
		}
		themes := make([]any, 0, len(registry.ThemeManifests())+len(installed))
		for _, manifest := range registry.ThemeManifests() {
			themes = append(themes, map[string]any{
				"source":   "builtin",
				"manifest": manifest,
			})
		}
		for _, entry := range installed {
			themes = append(themes, map[string]any{
				"source":   "installed",
				"root":     entry.Root,
				"manifest": entry.Manifest,
				"metadata": entry.Metadata,
			})
		}
		return writeJSON(struct {
			Themes []any `json:"themes"`
		}{
			Themes: themes,
		})
	case "inspect":
		fs := flag.NewFlagSet("themes inspect", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		dir := fs.String("dir", "", "theme directory containing haonews.theme.json")
		root := fs.String("root", "", "extensions root override")
		if err := parseFlagSetInterspersed(fs, args[1:]); err != nil {
			return err
		}
		if strings.TrimSpace(*dir) != "" {
			theme, err := directorytheme.Load(*dir)
			if err != nil {
				return err
			}
			return writeJSON(struct {
				Dir      string                `json:"dir"`
				Manifest apphost.ThemeManifest `json:"manifest"`
			}{
				Dir:      *dir,
				Manifest: theme.Manifest(),
			})
		}
		if fs.NArg() == 0 {
			return errors.New("theme id or --dir is required")
		}
		store, err := extensions.Open(*root)
		if err != nil {
			return err
		}
		if entry, err := store.GetTheme(fs.Arg(0)); err == nil {
			return writeJSON(struct {
				Source   string                     `json:"source"`
				Root     string                     `json:"root"`
				Manifest apphost.ThemeManifest      `json:"manifest"`
				Metadata extensions.InstallMetadata `json:"metadata"`
			}{
				Source:   "installed",
				Root:     entry.Root,
				Manifest: entry.Manifest,
				Metadata: entry.Metadata,
			})
		}
		registry := builtin.DefaultRegistry()
		_, manifest, err := registry.ResolveTheme(fs.Arg(0))
		if err != nil {
			return err
		}
		return writeJSON(struct {
			Source   string                `json:"source"`
			Manifest apphost.ThemeManifest `json:"manifest"`
		}{
			Source:   "builtin",
			Manifest: manifest,
		})
	case "install", "link":
		fs := flag.NewFlagSet("themes "+args[0], flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		dir := fs.String("dir", "", "theme directory containing haonews.theme.json")
		root := fs.String("root", "", "extensions root override")
		if err := parseFlagSetInterspersed(fs, args[1:]); err != nil {
			return err
		}
		store, err := extensions.Open(*root)
		if err != nil {
			return err
		}
		entry, err := store.InstallTheme(*dir, args[0] == "link")
		if err != nil {
			return err
		}
		return writeJSON(entry)
	case "remove":
		fs := flag.NewFlagSet("themes remove", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		root := fs.String("root", "", "extensions root override")
		if err := parseFlagSetInterspersed(fs, args[1:]); err != nil {
			return err
		}
		if fs.NArg() == 0 {
			return errors.New("theme id is required")
		}
		store, err := extensions.Open(*root)
		if err != nil {
			return err
		}
		if err := store.RemoveTheme(fs.Arg(0)); err != nil {
			return err
		}
		return writeJSON(map[string]any{"removed": fs.Arg(0)})
	default:
		return errors.New("usage: haonews themes <list|inspect|install|link|remove>")
	}
}

func manifestsToAny[T any](items []T) []any {
	out := make([]any, 0, len(items))
	for _, item := range items {
		out = append(out, item)
	}
	return out
}

func inspectAppDir(dir, extensionsRoot string) (workspace.AppBundle, workspace.ValidationReport, error) {
	bundle, err := workspace.LoadAppBundle(dir)
	if err != nil {
		return workspace.AppBundle{}, workspace.ValidationReport{}, err
	}
	registry := builtin.DefaultRegistry()
	store, err := extensions.Open(extensionsRoot)
	if err != nil {
		return workspace.AppBundle{}, workspace.ValidationReport{}, err
	}
	if _, err := store.RegisterIntoRegistry(registry, "", "", bundle.App.ID); err != nil {
		return workspace.AppBundle{}, workspace.ValidationReport{}, err
	}
	plugins, _, err := workspace.LoadPlugins(filepath.Join(bundle.Root, "plugins"), registry)
	if err != nil {
		return workspace.AppBundle{}, workspace.ValidationReport{}, err
	}
	for _, plugin := range plugins {
		if err := registry.RegisterPlugin(plugin); err != nil {
			return workspace.AppBundle{}, workspace.ValidationReport{}, err
		}
	}
	for _, theme := range bundle.Themes {
		if err := registry.RegisterTheme(theme); err != nil {
			return workspace.AppBundle{}, workspace.ValidationReport{}, err
		}
	}
	report, err := workspace.ValidateAppBundle(bundle, registry, registry)
	if err != nil {
		return workspace.AppBundle{}, workspace.ValidationReport{}, err
	}
	return bundle, report, nil
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func writeJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func usageError() error {
	return errors.New("usage: haonews <identity|credit|live|publish|verify|show|sync|serve|plugins|themes|apps|create> [flags]")
}

func loadJSONObject(inline, path string) (map[string]any, error) {
	inline = strings.TrimSpace(inline)
	path = strings.TrimSpace(path)
	if inline != "" && path != "" {
		return nil, errors.New("use only one of extensions-json or extensions-file")
	}
	if inline == "" && path == "" {
		return map[string]any{}, nil
	}
	var data []byte
	var err error
	if inline != "" {
		data = []byte(inline)
	} else {
		data, err = os.ReadFile(path)
		if err != nil {
			return nil, err
		}
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, fmt.Errorf("parse extensions json: %w", err)
	}
	if value == nil {
		value = map[string]any{}
	}
	return value, nil
}
