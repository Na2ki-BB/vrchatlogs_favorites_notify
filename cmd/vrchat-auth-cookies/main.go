package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	apiBaseURL  = "https://api.vrchat.cloud/api/1"
	maxBodySize = 1 << 20
)

type credentials struct {
	Username   string
	Password   string
	TOTPSecret string
	UserAgent  string
}

type cookiePair struct {
	Auth      string
	TwoFactor string
}

type authenticator struct {
	client  *http.Client
	baseURL string
	now     func() time.Time
}

type currentUserResponse struct {
	ID                    string   `json:"id"`
	RequiresTwoFactorAuth []string `json:"requiresTwoFactorAuth"`
}

type verify2FAResponse struct {
	Verified bool `json:"verified"`
}

type apiErrorResponse struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

type cookieOutputOptions struct {
	envFile     string
	updateEnv   bool
	interactive bool
	resultFD    int
}

func main() {
	log.SetFlags(0)

	envFile := flag.String("env-file", "/etc/vrchat-logger.env", "environment file containing VRChat credentials (empty: process environment only)")
	updateEnv := flag.Bool("update-env", false, "replace VRC_AUTH_TOKEN values in the environment file instead of printing them")
	interactive := flag.Bool("interactive", false, "ask before updating the environment file (default when no flags are given in a terminal)")
	resultFD := flag.Int("result-fd", -1, "write the non-secret interactive result to this inherited file descriptor")
	flag.Parse()
	if *updateEnv && *interactive {
		log.Fatal("-update-env and -interactive cannot be used together")
	}
	if *resultFD >= 0 && !*interactive {
		log.Fatal("-result-fd requires -interactive")
	}
	if *resultFD >= 0 && *resultFD < 3 {
		log.Fatal("-result-fd must be an inherited descriptor numbered 3 or greater")
	}

	flagWasSet := false
	flag.Visit(func(*flag.Flag) { flagWasSet = true })
	autoInteractive := !flagWasSet && terminalInput(os.Stdin)

	resolvedEnvFile := *envFile
	if resolvedEnvFile != "" {
		var err error
		resolvedEnvFile, err = resolveEnvironmentPath(resolvedEnvFile)
		if err != nil {
			log.Fatalf("resolve environment file %s: %v", *envFile, err)
		}
	}

	fileValues := map[string]string{}
	var err error
	if resolvedEnvFile != "" {
		fileValues, err = readEnvironmentFile(resolvedEnvFile)
		if err != nil {
			log.Fatalf("read environment file %s: %v", resolvedEnvFile, err)
		}
	}

	creds, err := credentialsFromSources(fileValues)
	if err != nil {
		log.Fatal(err)
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		log.Fatal("create cookie jar: ", err)
	}

	auth := authenticator{
		client:  &http.Client{Jar: jar, Timeout: 15 * time.Second},
		baseURL: apiBaseURL,
		now:     time.Now,
	}

	cookies, err := auth.authenticate(context.Background(), creds)
	if err != nil {
		log.Fatal(err)
	}

	if err := handleCookieOutput(
		os.Stdin,
		os.Stdout,
		os.Stderr,
		cookies,
		cookieOutputOptions{
			envFile:     resolvedEnvFile,
			updateEnv:   *updateEnv,
			interactive: *interactive || autoInteractive,
			resultFD:    *resultFD,
		},
		updateEnvironmentFile,
	); err != nil {
		log.Fatal(err)
	}
}

func resolveEnvironmentPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	return resolved, nil
}

func handleCookieOutput(
	in io.Reader,
	stdout io.Writer,
	stderr io.Writer,
	cookies cookiePair,
	options cookieOutputOptions,
	update func(string, cookiePair) error,
) error {
	if options.updateEnv {
		if options.envFile == "" {
			return errors.New("-update-env requires -env-file")
		}
		if err := update(options.envFile, cookies); err != nil {
			return fmt.Errorf("update environment file: %w", err)
		}
		fmt.Fprintf(stderr, "authentication succeeded; updated %s\n", options.envFile)
		return nil
	}

	if options.interactive {
		if options.envFile == "" {
			return errors.New("interactive update requires -env-file")
		}
		confirmed, err := promptYesNo(in, stderr, fmt.Sprintf("Update cookies in %s? [y/n]: ", options.envFile))
		if err != nil {
			return err
		}
		if confirmed {
			if err := update(options.envFile, cookies); err != nil {
				return fmt.Errorf("update environment file: %w", err)
			}
			if err := writeInteractiveResult(options.resultFD, "updated"); err != nil {
				return err
			}
			printCookies(stdout, cookies)
			fmt.Fprintf(stderr, "authentication succeeded; updated %s\n", options.envFile)
			return nil
		}
		if err := writeInteractiveResult(options.resultFD, "skipped"); err != nil {
			return err
		}
		printCookies(stdout, cookies)
		fmt.Fprintln(stderr, "authentication succeeded; environment file was not changed")
		fmt.Fprintln(stderr, "protect the cookie output like a password")
		return nil
	}

	// Keep stdout machine-readable. All diagnostics are written to stderr.
	printCookies(stdout, cookies)
	fmt.Fprintln(stderr, "authentication succeeded; protect the cookie output like a password")
	return nil
}

func promptYesNo(in io.Reader, out io.Writer, prompt string) (bool, error) {
	reader := bufio.NewReader(in)
	for {
		fmt.Fprint(out, prompt)
		line, err := reader.ReadString('\n')
		answer := strings.ToLower(strings.TrimSpace(line))
		if err != nil && !errors.Is(err, io.EOF) {
			return false, fmt.Errorf("read confirmation: %w", err)
		}
		switch answer {
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		case "":
			fmt.Fprintln(out, "no answer received; environment file will not be changed")
			return false, nil
		default:
			fmt.Fprintln(out, "please answer y/yes or n/no")
		}
		if errors.Is(err, io.EOF) {
			fmt.Fprintln(out, "end of input; environment file will not be changed")
			return false, nil
		}
	}
}

func writeInteractiveResult(fd int, result string) error {
	if fd < 0 {
		return nil
	}
	f := os.NewFile(uintptr(fd), "interactive-result")
	if f == nil {
		return fmt.Errorf("result file descriptor %d is unavailable", fd)
	}
	if _, err := io.WriteString(f, result+"\n"); err != nil {
		return fmt.Errorf("write interactive result: %w", err)
	}
	return nil
}

func credentialsFromSources(fileValues map[string]string) (credentials, error) {
	value := func(name string) string {
		if current, ok := os.LookupEnv(name); ok {
			return current
		}
		return fileValues[name]
	}

	creds := credentials{
		Username:   strings.TrimSpace(value("VRC_USERNAME")),
		Password:   value("VRC_PASSWORD"),
		TOTPSecret: strings.TrimSpace(value("VRC_TOTP_SECRET")),
		UserAgent:  strings.TrimSpace(value("VRC_USER_AGENT")),
	}

	var missing []string
	if creds.Username == "" {
		missing = append(missing, "VRC_USERNAME")
	}
	if creds.Password == "" {
		missing = append(missing, "VRC_PASSWORD")
	}
	if creds.TOTPSecret == "" {
		missing = append(missing, "VRC_TOTP_SECRET")
	}
	if creds.UserAgent == "" {
		missing = append(missing, "VRC_USER_AGENT")
	}
	if len(missing) != 0 {
		return credentials{}, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}

	return creds, nil
}

func readEnvironmentFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	values := make(map[string]string)
	scanner := bufio.NewScanner(f)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		key, value, ok, err := parseEnvironmentAssignment(scanner.Text())
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNumber, err)
		}
		if ok {
			values[key] = value
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

func parseEnvironmentAssignment(line string) (key, value string, ok bool, err error) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
		return "", "", false, nil
	}
	trimmed = strings.TrimPrefix(trimmed, "export ")
	parts := strings.SplitN(trimmed, "=", 2)
	if len(parts) != 2 {
		return "", "", false, errors.New("expected NAME=VALUE")
	}
	key = strings.TrimSpace(parts[0])
	if !validEnvironmentName(key) {
		return "", "", false, fmt.Errorf("invalid environment variable name %q", key)
	}

	value = strings.TrimSpace(parts[1])
	if len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'' {
		return key, value[1 : len(value)-1], true, nil
	}
	if strings.HasPrefix(value, "\"") {
		unquoted, unquoteErr := strconv.Unquote(value)
		if unquoteErr != nil {
			return "", "", false, fmt.Errorf("invalid quoted value for %s", key)
		}
		return key, unquoted, true, nil
	}
	return key, value, true, nil
}

func validEnvironmentName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' || (i > 0 && r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

func updateEnvironmentFile(path string, cookies cookiePair) error {
	return updateEnvironmentFileWithRename(path, cookies, os.Rename)
}

func updateEnvironmentFileWithRename(path string, cookies cookiePair, rename func(string, string) error) error {
	if strings.ContainsAny(cookies.Auth, "\r\n") || strings.ContainsAny(cookies.TwoFactor, "\r\n") {
		return errors.New("refusing to write a cookie containing a newline")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%s permissions are %04o; run chmod 600 first", path, info.Mode().Perm())
	}

	replacements := map[string]string{
		"VRC_AUTH_TOKEN":            cookies.Auth,
		"VRC_TWO_FACTOR_AUTH_TOKEN": cookies.TwoFactor,
	}
	found := make(map[string]bool)
	newline := "\n"
	if bytes.Contains(data, []byte("\r\n")) {
		newline = "\r\n"
	}
	var updated strings.Builder
	for len(data) > 0 {
		lineEnd := bytes.IndexByte(data, '\n')
		var line []byte
		if lineEnd < 0 {
			line = data
			data = nil
		} else {
			line = data[:lineEnd+1]
			data = data[lineEnd+1:]
		}

		contentEnd := len(line)
		if contentEnd > 0 && line[contentEnd-1] == '\n' {
			contentEnd--
			if contentEnd > 0 && line[contentEnd-1] == '\r' {
				contentEnd--
			}
		}
		content := string(line[:contentEnd])
		ending := string(line[contentEnd:])
		trimmed := strings.TrimSpace(content)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			updated.WriteString(string(line))
			continue
		}
		parts := strings.SplitN(strings.TrimPrefix(trimmed, "export "), "=", 2)
		if len(parts) != 2 {
			updated.WriteString(string(line))
			continue
		}
		key := strings.TrimSpace(parts[0])
		value, replace := replacements[key]
		if !replace {
			updated.WriteString(string(line))
			continue
		}
		equals := strings.Index(content, "=")
		updated.WriteString(content[:equals+1])
		updated.WriteString(value)
		updated.WriteString(ending)
		found[key] = true
	}
	missing := make([]string, 0, 2)
	for _, key := range []string{"VRC_AUTH_TOKEN", "VRC_TWO_FACTOR_AUTH_TOKEN"} {
		if !found[key] {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		if updated.Len() > 0 && !strings.HasSuffix(updated.String(), "\n") {
			updated.WriteString(newline)
		}
		for _, key := range missing {
			updated.WriteString(key + "=" + replacements[key] + newline)
		}
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if err := preserveFileOwnership(tmp, info); err != nil {
		tmp.Close()
		return err
	}
	if _, err := io.WriteString(tmp, updated.String()); err != nil {
		tmp.Close()
		return err
	}
	// Chown and file writes can clear set-id bits, so restore the complete
	// original mode only after ownership and content have been applied.
	if err := tmp.Chmod(info.Mode()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := rename(tmpName, path); err != nil {
		return err
	}
	return syncParentDirectory(path)
}

func printCookies(w io.Writer, cookies cookiePair) {
	fmt.Fprintf(w, "VRC_AUTH_TOKEN=%s\n", cookies.Auth)
	fmt.Fprintf(w, "VRC_TWO_FACTOR_AUTH_TOKEN=%s\n", cookies.TwoFactor)
}

func (a authenticator) authenticate(ctx context.Context, creds credentials) (cookiePair, error) {
	login, err := a.getCurrentUser(ctx, creds, true)
	if err != nil {
		return cookiePair{}, fmt.Errorf("VRChat login failed: %w", err)
	}

	authCookie := a.cookieValue("auth")
	if authCookie == "" {
		return cookiePair{}, errors.New("VRChat login did not return an auth cookie")
	}

	if contains(login.RequiresTwoFactorAuth, "totp") {
		code, err := totpCode(creds.TOTPSecret, a.now(), 6)
		if err != nil {
			return cookiePair{}, fmt.Errorf("generate TOTP code: %w", err)
		}
		if err := a.verifyTOTP(ctx, creds.UserAgent, code); err != nil {
			return cookiePair{}, err
		}
	} else if len(login.RequiresTwoFactorAuth) != 0 {
		return cookiePair{}, fmt.Errorf(
			"account requires unsupported MFA method(s): %s (TOTP is required for unattended authentication)",
			strings.Join(login.RequiresTwoFactorAuth, ", "),
		)
	}

	verified, err := a.getCurrentUser(ctx, creds, false)
	if err != nil {
		return cookiePair{}, fmt.Errorf("verify authenticated session: %w", err)
	}
	if len(verified.RequiresTwoFactorAuth) != 0 {
		return cookiePair{}, fmt.Errorf("MFA is still required after TOTP verification: %s", strings.Join(verified.RequiresTwoFactorAuth, ", "))
	}

	pair := cookiePair{
		Auth:      a.cookieValue("auth"),
		TwoFactor: a.cookieValue("twoFactorAuth"),
	}
	if pair.Auth == "" {
		return cookiePair{}, errors.New("authenticated session has no auth cookie")
	}
	if pair.TwoFactor == "" {
		return cookiePair{}, errors.New("TOTP verification succeeded but no twoFactorAuth cookie was returned")
	}

	return pair, nil
}

func (a authenticator) getCurrentUser(ctx context.Context, creds credentials, withBasicAuth bool) (currentUserResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+"/auth/user", nil)
	if err != nil {
		return currentUserResponse{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", creds.UserAgent)
	if withBasicAuth {
		req.Header.Set("Authorization", basicAuthorization(creds.Username, creds.Password))
	}

	res, err := a.client.Do(req)
	if err != nil {
		return currentUserResponse{}, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(io.LimitReader(res.Body, maxBodySize))
	if err != nil {
		return currentUserResponse{}, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return currentUserResponse{}, responseError(res.StatusCode, body)
	}

	var parsed currentUserResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return currentUserResponse{}, fmt.Errorf("decode response: %w", err)
	}
	return parsed, nil
}

func (a authenticator) verifyTOTP(ctx context.Context, userAgent, code string) error {
	payload, err := json.Marshal(map[string]string{"code": code})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		a.baseURL+"/auth/twofactorauth/totp/verify",
		strings.NewReader(string(payload)),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)

	res, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("TOTP verification failed: %w", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(io.LimitReader(res.Body, maxBodySize))
	if err != nil {
		return fmt.Errorf("read TOTP response: %w", err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("TOTP verification failed: %w", responseError(res.StatusCode, body))
	}

	var parsed verify2FAResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fmt.Errorf("decode TOTP response: %w", err)
	}
	if !parsed.Verified {
		return errors.New("VRChat did not confirm TOTP verification")
	}
	return nil
}

func (a authenticator) cookieValue(name string) string {
	u, err := url.Parse(a.baseURL)
	if err != nil {
		return ""
	}
	for _, cookie := range a.client.Jar.Cookies(u) {
		if cookie.Name == name {
			return cookie.Value
		}
	}
	return ""
}

func basicAuthorization(username, password string) string {
	encoded := encodeCredential(username) + ":" + encodeCredential(password)
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(encoded))
}

func encodeCredential(value string) string {
	// QueryEscape is almost the encoding VRChat documents, except that it uses
	// '+' for spaces. Use %20 so the result is also safe with URI decoders that
	// do not apply application/x-www-form-urlencoded rules.
	return strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
}

func totpCode(secret string, at time.Time, digits int) (string, error) {
	if digits < 1 || digits > 9 {
		return "", fmt.Errorf("unsupported TOTP digit count: %d", digits)
	}

	normalized := strings.ToUpper(secret)
	normalized = strings.ReplaceAll(normalized, " ", "")
	normalized = strings.TrimRight(normalized, "=")
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(normalized)
	if err != nil {
		return "", errors.New("VRC_TOTP_SECRET is not valid Base32")
	}
	if len(key) == 0 {
		return "", errors.New("VRC_TOTP_SECRET decoded to an empty key")
	}

	counter := uint64(at.Unix() / 30)
	var counterBytes [8]byte
	binary.BigEndian.PutUint64(counterBytes[:], counter)

	mac := hmac.New(sha1.New, key)
	_, _ = mac.Write(counterBytes[:])
	digest := mac.Sum(nil)
	offset := digest[len(digest)-1] & 0x0f
	value := binary.BigEndian.Uint32(digest[offset:offset+4]) & 0x7fffffff

	modulus := uint32(1)
	for range digits {
		modulus *= 10
	}
	return fmt.Sprintf("%0*d", digits, value%modulus), nil
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}

func responseError(status int, body []byte) error {
	var parsed apiErrorResponse
	if err := json.Unmarshal(body, &parsed); err == nil && parsed.Error.Message != "" {
		return fmt.Errorf("HTTP %d: %s", status, parsed.Error.Message)
	}
	return fmt.Errorf("HTTP %d", status)
}
