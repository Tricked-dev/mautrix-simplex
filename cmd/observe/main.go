package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/coder/websocket"
)

var corrN int

type Conn struct {
	ws   *websocket.Conn
	port string
}

func dial(port string) *Conn {
	ctx := context.Background()
	ws, _, err := websocket.Dial(ctx, "ws://localhost:"+port, nil)
	if err != nil {
		panic(fmt.Sprintf("dial %s: %v", port, err))
	}
	ws.SetReadLimit(100 * 1024 * 1024)
	return &Conn{ws: ws, port: port}
}

func (c *Conn) send(cmd string) json.RawMessage {
	corrN++
	id := fmt.Sprintf("%d", corrN)
	msg := map[string]interface{}{"corrId": id, "cmd": cmd}
	data, _ := json.Marshal(msg)
	ctx := context.Background()
	c.ws.Write(ctx, websocket.MessageText, data)
	for {
		readCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		_, resp, err := c.ws.Read(readCtx)
		cancel()
		if err != nil {
			fmt.Printf("[%s] read err: %v\n", c.port, err)
			return nil
		}
		var raw map[string]json.RawMessage
		json.Unmarshal(resp, &raw)
		if cid, ok := raw["corrId"]; ok && string(cid) == `"`+id+`"` {
			return raw["resp"]
		}
		var t struct {
			Type string `json:"type"`
		}
		if r, ok := raw["resp"]; ok {
			json.Unmarshal(r, &t)
			fmt.Printf("  [%s async] %s\n", c.port, t.Type)
		}
	}
}

// waitEvents drains async events for up to dur. Returns raw resp of first matching type.
func (c *Conn) waitEvents(dur time.Duration, wantTypes ...string) (string, json.RawMessage) {
	wantSet := make(map[string]bool)
	for _, w := range wantTypes {
		wantSet[w] = true
	}
	deadline := time.Now().Add(dur)
	for time.Now().Before(deadline) {
		remaining := time.Until(deadline)
		readCtx, cancel := context.WithTimeout(context.Background(), remaining+100*time.Millisecond)
		_, resp, err := c.ws.Read(readCtx)
		cancel()
		if err != nil {
			if strings.Contains(err.Error(), "context deadline exceeded") || strings.Contains(err.Error(), "DeadlineExceeded") {
				return "", nil
			}
			fmt.Printf("  [%s] read err: %v\n", c.port, err)
			return "", nil
		}
		var raw map[string]json.RawMessage
		json.Unmarshal(resp, &raw)
		var t struct {
			Type string `json:"type"`
		}
		if r, ok := raw["resp"]; ok {
			json.Unmarshal(r, &t)
			_, hasCorrID := raw["corrId"]
			tag := "EVENT"
			if hasCorrID {
				tag = "cmd-resp"
			}
			fmt.Printf("  [%s %s] %s\n", c.port, tag, t.Type)
			if !hasCorrID && wantSet[t.Type] {
				return t.Type, r
			}
		}
	}
	return "", nil
}

func pretty(data json.RawMessage) string {
	var v interface{}
	json.Unmarshal(data, &v)
	b, _ := json.MarshalIndent(v, "", "  ")
	s := string(b)
	if len(s) > 2000 {
		s = s[:2000] + "\n  ...(truncated)"
	}
	return s
}

func main() {
	// The address from the main profile DB (full simplex: URI decoded from conn_req_contact blob)
	mainAddress := "simplex:/contact#/?v=2-7&smp=smp%3A%2F%2Fhpq7_4gGJiilmz5Rf-CswuU5kZGkm_zOIooSw6yALRg%3D%40smp5.simplex.im%2FMdP7_vjyL0of8-z1IXLO4E4CMcp8QKRP%23%2F%3Fv%3D1-4%26dh%3DMCowBQYDK2VuAyEAfB2pd4WA1mHpdPCUtPHoMCVIUVt2ULLtGyPq9z1ObXg%253D%26q%3Dc%26srv%3Djjbyvoemxysm7qxap7m5d5m35jzv5qq6gnlv7s4rsn7tdwwmuqciwpid.onion"
	if len(os.Args) > 1 {
		mainAddress = os.Args[1]
	}

	fmt.Println("=== SimpleX Bridge E2E Inbound Test ===")
	fmt.Printf("Main profile address: %s\n\n", mainAddress)

	fmt.Println("[1] Connecting to session2 (5226)...")
	s2 := dial("5226")
	defer s2.ws.Close(websocket.StatusNormalClosure, "done")

	// Get session2 user
	resp := s2.send("/u")
	var s2User struct {
		User struct {
			UserID  int64 `json:"userId"`
			Profile struct {
				DisplayName string `json:"displayName"`
			} `json:"profile"`
		} `json:"user"`
	}
	json.Unmarshal(resp, &s2User)
	fmt.Printf("Session2 user: %s (id=%d)\n\n", s2User.User.Profile.DisplayName, s2User.User.UserID)

	// Check existing contacts
	fmt.Println("[2] Checking session2 contacts...")
	contactsResp := s2.send(fmt.Sprintf("/_contacts %d", s2User.User.UserID))
	var contacts struct {
		Contacts []struct {
			ContactID        int64  `json:"contactId"`
			LocalDisplayName string `json:"localDisplayName"`
		} `json:"contacts"`
	}
	json.Unmarshal(contactsResp, &contacts)

	var mainContactID int64
	for _, c := range contacts.Contacts {
		fmt.Printf("  contact %d: %s\n", c.ContactID, c.LocalDisplayName)
		mainContactID = c.ContactID // take the last one as candidate
	}

	if mainContactID == 0 {
		fmt.Println("\n[3] No contacts found — initiating connection to main profile...")
		// Correct format: /_connect <userId> incognito=off <link>
		connectCmd := fmt.Sprintf("/_connect %d incognito=off %s", s2User.User.UserID, mainAddress)
		connectResp := s2.send(connectCmd)
		fmt.Printf("  connect response:\n%s\n", pretty(connectResp))

		fmt.Println("\n[4] Waiting up to 45s for contactConnected on session2...")
		evtType, evtRaw := s2.waitEvents(45*time.Second, "contactConnected", "contact")
		if evtType != "" {
			fmt.Printf("  Got event: %s\n%s\n", evtType, pretty(evtRaw))
			// Re-check contacts
			contactsResp = s2.send(fmt.Sprintf("/_contacts %d", s2User.User.UserID))
			json.Unmarshal(contactsResp, &contacts)
			for _, c := range contacts.Contacts {
				fmt.Printf("  contact %d: %s\n", c.ContactID, c.LocalDisplayName)
				mainContactID = c.ContactID
			}
		} else {
			fmt.Println("  No contactConnected event — the SMP relay may be slow.")
			fmt.Println("  Checking if any contact appeared anyway...")
			contactsResp = s2.send(fmt.Sprintf("/_contacts %d", s2User.User.UserID))
			json.Unmarshal(contactsResp, &contacts)
			for _, c := range contacts.Contacts {
				fmt.Printf("  contact %d: %s\n", c.ContactID, c.LocalDisplayName)
				mainContactID = c.ContactID
			}
		}
	}

	if mainContactID == 0 {
		fmt.Println("\nERROR: No contact to main profile available. Cannot send test message.")
		fmt.Println("The connection may still be pending (SMP relay). Try again in a few minutes.")
		os.Exit(1)
	}

	// Send a test message from session2 to main profile
	testMsg := fmt.Sprintf("Bridge inbound test at %s", time.Now().Format("15:04:05"))
	fmt.Printf("\n[5] Sending test message to contactId=%d: %q\n", mainContactID, testMsg)
	sendCmd := fmt.Sprintf(`/_send @%d live=off json [{"msgContent":{"type":"text","text":%s},"mentions":{}}]`,
		mainContactID, jsonStr(testMsg))
	sendResp := s2.send(sendCmd)
	var sendResult struct {
		Type      string `json:"type"`
		ChatItems []struct {
			ChatItem struct {
				Meta struct {
					ItemID   int64  `json:"itemId"`
					ItemText string `json:"itemText"`
				} `json:"meta"`
			} `json:"chatItem"`
		} `json:"chatItems"`
	}
	json.Unmarshal(sendResp, &sendResult)
	fmt.Printf("  send result: type=%s\n", sendResult.Type)
	if len(sendResult.ChatItems) > 0 {
		meta := sendResult.ChatItems[0].ChatItem.Meta
		fmt.Printf("  sent: itemId=%d text=%q\n", meta.ItemID, meta.ItemText)
	} else {
		fmt.Printf("  raw send response: %s\n", pretty(sendResp))
	}

	fmt.Println("\n[6] Now check bridge log and Matrix room for the inbound message.")
	fmt.Println("    Bridge log: /tmp/bridge.log")
	fmt.Println("    Matrix DM room: !ypiAzImngaq89uh0hV:localhost  (if session2 is contact 4)")
	fmt.Println("    A new portal room may be created for the new contact.")
	fmt.Println()
	fmt.Println("    Waiting 5s for any late async events on session2...")
	s2.waitEvents(5 * time.Second)

	fmt.Println("\n=== Done ===")
}

func jsonStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
