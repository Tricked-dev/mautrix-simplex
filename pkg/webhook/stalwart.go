package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type StalwartClient struct {
	Config    StalwartConfig
	accountID string
	mu        sync.RWMutex
}

type jmapResponse struct {
	MethodResponses [][]any `json:"methodResponses"`
}

type jmapSession struct {
	PrimaryAccounts map[string]string `json:"primaryAccounts"`
}

func (s *StalwartClient) getPassword() (string, error) {
	if s.Config.Pass != "" {
		return s.Config.Pass, nil
	}
	if s.Config.PassFile != "" {
		data, err := os.ReadFile(s.Config.PassFile)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(data)), nil
	}
	return "", nil
}

func (s *StalwartClient) getAccountID(ctx context.Context) (string, error) {
	s.mu.RLock()
	if s.accountID != "" {
		defer s.mu.RUnlock()
		return s.accountID, nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	// Double check after acquiring lock
	if s.accountID != "" {
		return s.accountID, nil
	}

	url := strings.TrimSuffix(s.Config.URL, "/") + "/jmap/session"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	pass, err := s.getPassword()
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(s.Config.User, pass)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code fetching session: %d", resp.StatusCode)
	}

	var session jmapSession
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return "", err
	}

	accID, ok := session.PrimaryAccounts["urn:ietf:params:jmap:mail"]
	if !ok {
		return "", fmt.Errorf("no mail account found in JMAP session")
	}

	s.accountID = accID
	return s.accountID, nil
}

func (s *StalwartClient) jmapRequest(ctx context.Context, methodCalls []any) (*jmapResponse, error) {
	payload := map[string]any{
		"using":       []string{"urn:ietf:params:jmap:core", "urn:ietf:params:jmap:mail"},
		"methodCalls": methodCalls,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	url := strings.TrimSuffix(s.Config.URL, "/") + "/jmap"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	pass, err := s.getPassword()
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(s.Config.User, pass)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code from JMAP: %d", resp.StatusCode)
	}

	var jResp jmapResponse
	if err := json.NewDecoder(resp.Body).Decode(&jResp); err != nil {
		return nil, err
	}

	return &jResp, nil
}

type emailAddress struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

func formatAddresses(addrs []emailAddress) string {
	var parts []string
	for _, a := range addrs {
		if a.Name != "" {
			parts = append(parts, fmt.Sprintf("%s <%s>", a.Name, a.Email))
		} else {
			parts = append(parts, a.Email)
		}
	}
	return strings.Join(parts, ", ")
}

func (s *StalwartClient) FetchEmailMetadata(ctx context.Context, messageID string) (map[string]any, time.Time, error) {
	var lastErr error
	for i := 0; i < 3; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
				return nil, time.Time{}, ctx.Err()
			case <-time.After(2 * time.Second):
			}
		}

		accID, err := s.getAccountID(ctx)
		if err != nil {
			lastErr = err
			continue
		}

		// Replicating Python logic: query 10 most recent and look for match
		methodCalls := []any{
			[]any{"Email/query", map[string]any{
				"accountId": accID,
				"sort":      []any{map[string]any{"property": "receivedAt", "isAscending": false}},
				"limit":     10,
			}, "q1"},
			[]any{"Email/get", map[string]any{
				"accountId": accID,
				"#ids": map[string]any{
					"resultOf": "q1",
					"name":     "Email/query",
					"path":     "/ids",
				},
				"properties": []string{"id", "messageId", "subject", "from", "to", "preview", "receivedAt"},
			}, "g1"},
		}

		resp, err := s.jmapRequest(ctx, methodCalls)
		if err != nil {
			lastErr = err
			continue
		}

		var emails []map[string]any
		for _, mr := range resp.MethodResponses {
			if len(mr) < 2 {
				continue
			}
			methodName, ok := mr[0].(string)
			if !ok {
				continue
			}
			if methodName == "error" {
				lastErr = fmt.Errorf("JMAP error: %v", mr[1])
				continue
			}
			if methodName == "Email/get" {
				args, ok := mr[1].(map[string]any)
				if !ok {
					continue
				}
				list, ok := args["list"].([]any)
				if !ok {
					continue
				}
				for _, item := range list {
					if m, ok := item.(map[string]any); ok {
						emails = append(emails, m)
					}
				}
			}
		}

		for _, email := range emails {
			mID, _ := email["messageId"].([]any)
			found := false
			for _, midVal := range mID {
				midStr, ok := midVal.(string)
				if !ok {
					continue
				}
				if midStr == messageID || midStr == "<"+messageID+">" || strings.Trim(midStr, "<>") == strings.Trim(messageID, "<>") {
					found = true
					break
				}
			}

			if found {
				receivedAtStr, _ := email["receivedAt"].(string)
				receivedAt, err := time.Parse(time.RFC3339, receivedAtStr)
				if err != nil {
					// Fallback to now if parsing fails, though JMAP should be RFC3339
					receivedAt = time.Now()
				}

				from, _ := email["from"].([]any)
				to, _ := email["to"].([]any)

				parsedFrom := parseEmailAddresses(from)
				parsedTo := parseEmailAddresses(to)

				metadata := map[string]any{
					"subject":    email["subject"],
					"from":       formatAddresses(parsedFrom),
					"to":         formatAddresses(parsedTo),
					"preview":    email["preview"],
					"receivedAt": receivedAt,
				}
				return metadata, receivedAt, nil
			}
		}
		lastErr = fmt.Errorf("email with messageId %s not found in recent emails", messageID)
	}

	return nil, time.Time{}, lastErr
}

func parseEmailAddresses(input []any) []emailAddress {
	var addrs []emailAddress
	for _, item := range input {
		m := item.(map[string]any)
		addr := emailAddress{}
		if name, ok := m["name"].(string); ok {
			addr.Name = name
		}
		if email, ok := m["email"].(string); ok {
			addr.Email = email
		}
		addrs = append(addrs, addr)
	}
	return addrs
}
