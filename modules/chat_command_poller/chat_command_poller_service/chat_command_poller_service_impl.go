package chat_command_poller_service

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"jira-u-bot/base/helpers/curl_helper"
	"jira-u-bot/modules/chat_command_poll_cursor/chat_command_poll_cursor_repository"
	"jira-u-bot/modules/chat_interactive/chat_interactive_service"
	"jira-u-bot/modules/chat_oauth/chat_oauth_service"
	"jira-u-bot/modules/registration/registration_dto"
	"jira-u-bot/modules/registration/registration_repository"
)

const pollInterval = 10 * time.Second

// commandPrefix gates which incoming messages are even considered for
// routing — unlike the old Chat-App-event trigger (where every delivered
// event was inherently a deliberate command attempt), a DM space polled
// this way is an ordinary human conversation. Only text starting with this
// prefix is treated as a command attempt; everything else is left alone so
// casual chat never gets an unsolicited bot reply.
const commandPrefix = "/u-"

// chatAPIBase is a package-level var (test seam), same pattern as
// chat_dm_notifier_service's chatAPIBase.
var chatAPIBase = "https://chat.googleapis.com/v1"

type chatCommandPollerServiceImpl struct {
	oauth            chat_oauth_service.ChatOAuthService
	registrationRepo registration_repository.RegistrationRepository
	cursorRepo       chat_command_poll_cursor_repository.ChatCommandPollCursorRepository
	chatInteractive  chat_interactive_service.ChatInteractiveService
	dmSender         DMSender
	selfUserID       string
}

func NewChatCommandPollerService(
	oauth chat_oauth_service.ChatOAuthService,
	registrationRepo registration_repository.RegistrationRepository,
	cursorRepo chat_command_poll_cursor_repository.ChatCommandPollCursorRepository,
	chatInteractive chat_interactive_service.ChatInteractiveService,
	dmSender DMSender,
	selfUserID string,
) ChatCommandPollerService {
	return &chatCommandPollerServiceImpl{
		oauth:            oauth,
		registrationRepo: registrationRepo,
		cursorRepo:       cursorRepo,
		chatInteractive:  chatInteractive,
		dmSender:         dmSender,
		selfUserID:       selfUserID,
	}
}

func (p *chatCommandPollerServiceImpl) Run(ctx context.Context) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.pollOnce(ctx); err != nil {
				log.Printf("chat_command_poller: cycle failed: %v", err)
			}
		}
	}
}

func (p *chatCommandPollerServiceImpl) pollOnce(ctx context.Context) error {
	users, err := p.registrationRepo.FindAllWithGoogleChatDMSpaceID(ctx)
	if err != nil {
		return fmt.Errorf("chat_command_poller: FindAllWithGoogleChatDMSpaceID failed: %w", err)
	}

	for _, user := range users {
		p.processSpace(ctx, user)
	}
	return nil
}

func (p *chatCommandPollerServiceImpl) processSpace(ctx context.Context, user registration_dto.User) {
	space := user.GoogleChatDMSpaceID

	cursor, cursorFound, err := p.cursorRepo.GetCursor(ctx, space)
	if err != nil {
		log.Printf("chat_command_poller: GetCursor(%s) failed: %v", space, err)
		return
	}

	messages, err := p.listMessages(ctx, space)
	if err != nil {
		log.Printf("chat_command_poller: listMessages(%s) failed: %v", space, err)
		return
	}
	if len(messages) == 0 {
		return
	}

	sort.Slice(messages, func(i, j int) bool { return messages[i].createTime.Before(messages[j].createTime) })

	latest := cursor
	for _, msg := range messages {
		if msg.createTime.After(latest) {
			latest = msg.createTime
		}

		switch {
		case !cursorFound:
			// First-ever poll for this space: never replay pre-existing
			// history as commands, just establish the starting point.
		case !msg.createTime.After(cursor):
			// Already processed in a previous cycle.
		case msg.senderName == p.selfUserID:
			// Our own sent message (a prior reply, or the operator typing).
		case !strings.HasPrefix(strings.TrimSpace(msg.text), commandPrefix):
			// Ordinary conversation — stay silent.
		default:
			reply := p.chatInteractive.RouteCommand(ctx, user.Email, strings.TrimSpace(msg.text))
			if err := p.dmSender.SendPlainTextDM(ctx, user.Email, "🤖 "+reply); err != nil {
				log.Printf("chat_command_poller: SendPlainTextDM(%s) failed: %v", user.Email, err)
			}
		}
	}

	if err := p.cursorRepo.UpdateCursor(ctx, space, latest); err != nil {
		log.Printf("chat_command_poller: UpdateCursor(%s) failed: %v", space, err)
	}
}

type parsedMessage struct {
	senderName string
	text       string
	createTime time.Time
}

type listMessagesResponse struct {
	Messages []struct {
		Name   string `json:"name"`
		Sender struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"sender"`
		CreateTime string `json:"createTime"`
		Text       string `json:"text"`
	} `json:"messages"`
}

func (p *chatCommandPollerServiceImpl) listMessages(ctx context.Context, spaceID string) ([]parsedMessage, error) {
	token, err := p.oauth.GetAccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("chat_command_poller: GetAccessToken failed: %w", err)
	}

	q := url.Values{}
	q.Set("pageSize", "10")
	q.Set("orderBy", "createTime desc")
	reqURL := chatAPIBase + "/" + spaceID + "/messages?" + q.Encode()

	resp, err := curl_helper.CurlRequest[listMessagesResponse](
		ctx, reqURL,
		map[string]string{"Authorization": "Bearer " + token},
		nil, http.MethodGet, 10000, true,
	)
	if err != nil {
		return nil, fmt.Errorf("chat_command_poller: list messages for %s failed: %w", spaceID, err)
	}

	parsed := make([]parsedMessage, 0, len(resp.Messages))
	for _, m := range resp.Messages {
		t, err := time.Parse(time.RFC3339Nano, m.CreateTime)
		if err != nil {
			log.Printf("chat_command_poller: unparseable createTime %q on message %s, skipping: %v", m.CreateTime, m.Name, err)
			continue
		}
		parsed = append(parsed, parsedMessage{senderName: m.Sender.Name, text: m.Text, createTime: t})
	}
	return parsed, nil
}
