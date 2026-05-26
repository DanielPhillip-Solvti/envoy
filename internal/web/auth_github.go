package web

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/example/staccato/internal/platform"
)

const (
	sessionCookieName    = "staccato_session"
	oauthStateCookieName = "staccato_oauth_state"
)

type authUser struct {
	Login string
	Name  string
}

type authSession struct {
	ID          string
	User        authUser
	AccessToken string
	ExpiresAt   time.Time
}

type authManager struct {
	secureCookie bool
	mu           sync.RWMutex
	sessions     map[string]authSession
}

func newAuthManager(secureCookie bool) *authManager {
	return &authManager{
		secureCookie: secureCookie,
		sessions:     map[string]authSession{},
	}
}

func (a *authManager) create(w http.ResponseWriter, user authUser, accessToken string) (authSession, error) {
	sessionID, err := randomToken(32)
	if err != nil {
		return authSession{}, err
	}
	session := authSession{
		ID:          sessionID,
		User:        user,
		AccessToken: accessToken,
		ExpiresAt:   time.Now().Add(24 * time.Hour),
	}
	a.mu.Lock()
	a.sessions[sessionID] = session
	a.mu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   a.secureCookie,
		SameSite: http.SameSiteLaxMode,
		Expires:  session.ExpiresAt,
	})
	return session, nil
}

func (a *authManager) get(r *http.Request) (authSession, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return authSession{}, false
	}
	a.mu.RLock()
	session, ok := a.sessions[cookie.Value]
	a.mu.RUnlock()
	if !ok {
		return authSession{}, false
	}
	if time.Now().After(session.ExpiresAt) {
		a.mu.Lock()
		delete(a.sessions, session.ID)
		a.mu.Unlock()
		return authSession{}, false
	}
	return session, true
}

func (a *authManager) clear(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil && cookie.Value != "" {
		a.mu.Lock()
		delete(a.sessions, cookie.Value)
		a.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   a.secureCookie,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

type githubClient struct {
	clientID     string
	clientSecret string
	callbackURL  string
	httpClient   *http.Client
}

func newGitHubClient(clientID, clientSecret, callbackURL string) *githubClient {
	return &githubClient{
		clientID:     strings.TrimSpace(clientID),
		clientSecret: strings.TrimSpace(clientSecret),
		callbackURL:  strings.TrimSpace(callbackURL),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (g *githubClient) configured() bool {
	return g.clientID != "" && g.clientSecret != "" && g.callbackURL != ""
}

func (g *githubClient) loginURL(state string) string {
	q := url.Values{}
	q.Set("client_id", g.clientID)
	q.Set("redirect_uri", g.callbackURL)
	q.Set("scope", "read:user repo")
	q.Set("state", state)
	return "https://github.com/login/oauth/authorize?" + q.Encode()
}

func (g *githubClient) exchangeCode(ctx context.Context, code string) (string, error) {
	form := url.Values{}
	form.Set("client_id", g.clientID)
	form.Set("client_secret", g.clientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", g.callbackURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://github.com/login/oauth/access_token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("oauth exchange failed: %s", strings.TrimSpace(string(body)))
	}

	var payload struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if payload.Error != "" {
		return "", errors.New(payload.Error)
	}
	if payload.AccessToken == "" {
		return "", errors.New("missing access token")
	}
	return payload.AccessToken, nil
}

func (g *githubClient) currentUser(ctx context.Context, token string) (authUser, error) {
	var payload struct {
		Login string `json:"login"`
		Name  string `json:"name"`
	}
	if err := g.apiGet(ctx, token, "/user", &payload); err != nil {
		return authUser{}, err
	}
	if payload.Login == "" {
		return authUser{}, errors.New("missing GitHub user login")
	}
	return authUser{Login: payload.Login, Name: payload.Name}, nil
}

func (g *githubClient) hasRepoAccess(ctx context.Context, token, repo string) (bool, error) {
	owner, name, err := parseGitHubRepo(repo)
	if err != nil {
		return false, err
	}
	endpoint := "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(name)
	var out map[string]any
	err = g.apiGet(ctx, token, endpoint, &out)
	if err == nil {
		return true, nil
	}
	if strings.Contains(err.Error(), "status 404") || strings.Contains(err.Error(), "status 403") {
		return false, nil
	}
	return false, err
}

func (g *githubClient) commitHistory(ctx context.Context, token, repo, branch string, limit int) ([]commitView, error) {
	owner, name, err := parseGitHubRepo(repo)
	if err != nil {
		return nil, err
	}
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return nil, errors.New("environment is empty")
	}
	branchEndpoint := "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(name) + "/branches/" + url.PathEscape(branch)
	var branchCheck map[string]any
	if err := g.apiGet(ctx, token, branchEndpoint, &branchCheck); err != nil {
		if strings.Contains(err.Error(), "status 404") {
			return nil, fmt.Errorf("branch %q does not exist on GitHub", branch)
		}
		return nil, err
	}

	if limit <= 0 || limit > 100 {
		limit = 25
	}
	q := url.Values{}
	q.Set("sha", branch)
	q.Set("per_page", fmt.Sprintf("%d", limit))
	commitsEndpoint := "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(name) + "/commits?" + q.Encode()

	var payload []struct {
		SHA     string `json:"sha"`
		HTMLURL string `json:"html_url"`
		Commit  struct {
			Message string `json:"message"`
			Author  struct {
				Name string    `json:"name"`
				Date time.Time `json:"date"`
			} `json:"author"`
		} `json:"commit"`
	}
	if err := g.apiGet(ctx, token, commitsEndpoint, &payload); err != nil {
		return nil, err
	}

	result := make([]commitView, 0, len(payload))
	for _, item := range payload {
		message := strings.TrimSpace(item.Commit.Message)
		if idx := strings.Index(message, "\n"); idx > -1 {
			message = strings.TrimSpace(message[:idx])
		}
		sha := item.SHA
		shortSHA := sha
		if len(shortSHA) > 7 {
			shortSHA = shortSHA[:7]
		}
		result = append(result, commitView{
			SHA:       sha,
			ShortSHA:  shortSHA,
			Message:   message,
			Author:    item.Commit.Author.Name,
			PushedAt:  item.Commit.Author.Date,
			CommitURL: item.HTMLURL,
		})
	}
	return result, nil
}

func (g *githubClient) apiGet(ctx context.Context, token, endpoint string, out any) error {
	u := "https://api.github.com" + endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("github api status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (s *Server) startGitHubLogin(w http.ResponseWriter, r *http.Request) {
	if !s.gh.configured() {
		http.Error(w, "GitHub OAuth is not configured", http.StatusFailedDependency)
		return
	}
	state, err := randomToken(24)
	if err != nil {
		http.Error(w, "failed to create oauth state", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookieName,
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.auth.secureCookie,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   300,
	})
	http.Redirect(w, r, s.gh.loginURL(state), http.StatusFound)
}

func (s *Server) handleGitHubCallback(w http.ResponseWriter, r *http.Request) {
	if !s.gh.configured() {
		http.Error(w, "GitHub OAuth is not configured", http.StatusFailedDependency)
		return
	}
	if errText := r.URL.Query().Get("error"); errText != "" {
		http.Error(w, "GitHub OAuth error: "+errText, http.StatusUnauthorized)
		return
	}
	state := r.URL.Query().Get("state")
	cookie, err := r.Cookie(oauthStateCookieName)
	if err != nil || cookie.Value == "" || state == "" || state != cookie.Value {
		http.Error(w, "invalid oauth state", http.StatusUnauthorized)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing oauth code", http.StatusBadRequest)
		return
	}

	token, err := s.gh.exchangeCode(r.Context(), code)
	if err != nil {
		http.Error(w, "oauth token exchange failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	user, err := s.gh.currentUser(r.Context(), token)
	if err != nil {
		http.Error(w, "failed to fetch github user: "+err.Error(), http.StatusBadGateway)
		return
	}
	if _, err := s.auth.create(w, user, token); err != nil {
		http.Error(w, "failed to create session", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.auth.secureCookie,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	s.auth.clear(w, r)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) requireAuth(w http.ResponseWriter, r *http.Request) (authSession, bool) {
	session, ok := s.auth.get(r)
	if ok {
		return session, true
	}
	if isHX(r) {
		w.Header().Set("HX-Redirect", "/auth/github/login")
		w.WriteHeader(http.StatusUnauthorized)
		return authSession{}, false
	}
	http.Redirect(w, r, "/auth/github/login", http.StatusFound)
	return authSession{}, false
}

func (s *Server) canViewAgent(ctx context.Context, accessToken string, agent platform.AgentView) bool {
	repo := strings.TrimSpace(agent.Registration.Repo)
	if repo == "" {
		return true
	}
	ok, err := s.gh.hasRepoAccess(ctx, accessToken, repo)
	if err != nil {
		return false
	}
	return ok
}

func parseGitHubRepo(repo string) (string, string, error) {
	repo = strings.TrimSpace(repo)
	if repo == "" {
		return "", "", errors.New("repo is empty")
	}
	if strings.HasPrefix(repo, "git@github.com:") {
		clean := strings.Trim(strings.TrimPrefix(repo, "git@github.com:"), "/")
		clean = strings.Trim(strings.TrimSuffix(clean, ".git"), "/")
		parts := strings.Split(clean, "/")
		if len(parts) != 2 {
			return "", "", errors.New("repo must be in owner/repo format")
		}
		return parts[0], parts[1], nil
	}
	if strings.HasPrefix(repo, "ssh://") {
		u, err := url.Parse(repo)
		if err != nil {
			return "", "", err
		}
		if !strings.Contains(strings.ToLower(u.Host), "github.com") {
			return "", "", errors.New("repo must be on github.com")
		}
		clean := strings.Trim(strings.TrimSuffix(u.Path, ".git"), "/")
		parts := strings.Split(clean, "/")
		if len(parts) != 2 {
			return "", "", errors.New("repo must be in owner/repo format")
		}
		return parts[0], parts[1], nil
	}
	if strings.HasPrefix(repo, "http://") || strings.HasPrefix(repo, "https://") {
		u, err := url.Parse(repo)
		if err != nil {
			return "", "", err
		}
		if !strings.Contains(strings.ToLower(u.Host), "github.com") {
			return "", "", errors.New("repo must be on github.com")
		}
		clean := strings.Trim(strings.TrimSuffix(u.Path, ".git"), "/")
		parts := strings.Split(clean, "/")
		if len(parts) < 2 {
			return "", "", errors.New("invalid GitHub repo path")
		}
		return parts[0], path.Base(parts[1]), nil
	}
	clean := strings.Trim(strings.TrimSuffix(repo, ".git"), "/")
	parts := strings.Split(clean, "/")
	if len(parts) != 2 {
		return "", "", errors.New("repo must be in owner/repo format")
	}
	return parts[0], parts[1], nil
}

func randomToken(size int) (string, error) {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
