// Package kcadmin talks to Keycloak's admin REST API with vuln-api's own
// service-account client (D16): it lists users with their app role and
// changes app roles. It only ever touches the four VulnMan realm roles.
package kcadmin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/RaiseOfEASV/Cerberus/apps/vuln-api/internal/domain"
)

type Client struct {
	base, realm, clientID, secret string
	hc                            *http.Client

	mu      sync.Mutex
	token   string
	expires time.Time
}

func New(baseURL, realm, clientID, secret string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{base: strings.TrimRight(baseURL, "/"), realm: realm, clientID: clientID, secret: secret, hc: hc}
}

var userIDRe = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)

var errUnauthorized = errors.New("keycloak: unauthorized")

func (c *Client) adminToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().Before(c.expires) {
		return c.token, nil
	}
	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {c.clientID}, "client_secret": {c.secret}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/realms/"+url.PathEscape(c.realm)+"/protocol/openid-connect/token",
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("keycloak token: HTTP %d", resp.StatusCode)
	}
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return "", err
	}
	c.token = out.AccessToken
	c.expires = time.Now().Add(time.Duration(float64(out.ExpiresIn)*0.8) * time.Second)
	return c.token, nil
}

// do calls the admin API, fetching a fresh token once on 401.
func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	for attempt := 0; ; attempt++ {
		tok, err := c.adminToken(ctx)
		if err != nil {
			return err
		}
		err = c.once(ctx, method, path, tok, in, out)
		if errors.Is(err, errUnauthorized) && attempt == 0 {
			c.mu.Lock()
			c.token = ""
			c.mu.Unlock()
			continue
		}
		return err
	}
}

func (c *Client) once(ctx context.Context, method, path, tok string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+"/admin/realms/"+url.PathEscape(c.realm)+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return errUnauthorized
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case resp.StatusCode/100 != 2:
		return fmt.Errorf("keycloak %s %s: HTTP %d", method, path, resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(out)
}

var ErrNotFound = errors.New("keycloak: not found")

type kcUser struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Email     string `json:"email"`
}

func (u kcUser) person() domain.Person {
	name := strings.TrimSpace(u.FirstName + " " + u.LastName)
	if name == "" {
		name = u.Username
	}
	return domain.Person{Sub: u.ID, Username: u.Username, Name: name, Email: u.Email}
}

// ListUsers returns every realm user with their highest app role.
func (c *Client) ListUsers(ctx context.Context) ([]domain.Person, error) {
	var all []kcUser
	if err := c.do(ctx, http.MethodGet, "/users?max=1000&briefRepresentation=true", nil, &all); err != nil {
		return nil, err
	}
	people := map[string]*domain.Person{}
	order := []string{}
	for _, u := range all {
		p := u.person()
		people[u.ID] = &p
		order = append(order, u.ID)
	}
	// domain.Roles is highest first, so the first role seen wins.
	for _, r := range domain.Roles {
		var members []kcUser
		if err := c.do(ctx, http.MethodGet, "/roles/"+url.PathEscape(string(r))+"/users?max=1000", nil, &members); err != nil {
			return nil, err
		}
		for _, m := range members {
			if p, ok := people[m.ID]; ok && p.Role == "" {
				p.Role = r
			}
		}
	}
	out := make([]domain.Person, 0, len(order))
	for _, id := range order {
		out = append(out, *people[id])
	}
	return out, nil
}

// ListAssignees: users who can own work (analyst, developer, pm).
func (c *Client) ListAssignees(ctx context.Context) ([]domain.Person, error) {
	all, err := c.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	var out []domain.Person
	for _, p := range all {
		if p.Role == domain.RoleAnalyst || p.Role == domain.RoleDeveloper || p.Role == domain.RolePM {
			out = append(out, p)
		}
	}
	return out, nil
}

type roleRep struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (c *Client) role(ctx context.Context, r domain.Role) (roleRep, error) {
	var rep roleRep
	err := c.do(ctx, http.MethodGet, "/roles/"+url.PathEscape(string(r)), nil, &rep)
	return rep, err
}

// SetRole makes r the user's only app role: removes the other three, adds r.
// Other realm roles (offline_access etc.) are left alone.
func (c *Client) SetRole(ctx context.Context, sub string, r domain.Role) error {
	if _, err := domain.ParseRole(string(r)); err != nil {
		return err
	}
	if !userIDRe.MatchString(sub) {
		return fmt.Errorf("invalid user id")
	}
	var remove []roleRep
	for _, other := range domain.Roles {
		if other == r {
			continue
		}
		rep, err := c.role(ctx, other)
		if err != nil {
			return err
		}
		remove = append(remove, rep)
	}
	add, err := c.role(ctx, r)
	if err != nil {
		return err
	}
	path := "/users/" + url.PathEscape(sub) + "/role-mappings/realm"
	if err := c.do(ctx, http.MethodDelete, path, remove, nil); err != nil {
		return err
	}
	return c.do(ctx, http.MethodPost, path, []roleRep{add}, nil)
}
