package main

import "encoding/json"
import "fmt"
import "html"
import "io/ioutil"
import "log"
import "net/http"
import "net/url"
import "regexp"
import "strconv"
import "strings"
import "time"

import "github.com/BurntSushi/toml"
import "golang.org/x/net/websocket"

import "slackv/console"

//==============================
// config structures
//==============================

type Config struct {
	General      ConfigGeneral
	Notification ConfigNotification
}

type ConfigGeneral struct {
	Token string
}

type ConfigNotification struct {
	Patterns     []string
	MuteChannels []string `toml:"mute-channels"`
	MuteUsers    []string `toml:"mute-users"`
}

//==============================
// Slack structures
//==============================

//! @see https://api.slack.com/methods/rtm.start
type Token struct {
	Token string
}

type SlackUser struct {
	Id   string
	Name string
}

type SlackTeam struct {
	Id   string
	Name string
}

//! @see https://api.slack.com/types/channel
type SlackChannel struct {
	Id        string `json:"id"`
	Name      string `json:"name"`
	IsMember  bool   `json:"is_member"`
	IsPrivate bool   `json:"is_private"`
}

//! @see https://api.slack.com/types/group
type SlackGroup struct {
	Id         string   `json:"id"`
	Name       string   `json:"name"`
	IsArchived bool     `json:"is_archived"`
	Members    []string `json:"members"`
}

//! multiparty IM
//!
//! @see https://api.slack.com/types/mpim
type SlackMpim struct {
	Id      string
	Name    string
	Members []string
}

//! @see https://api.slack.com/types/im
type SlackIm struct {
	Id     string `json:"id"`
	UserId string `json:"user"`
}

type SlackBot struct {
	Id   string
	Name string
}

type SlackSession struct {
	Ok       bool
	Url      string
	Self     SlackUser
	Team     SlackTeam
	Users    []SlackUser
	Channels []SlackChannel
	Groups   []SlackGroup
	Mpims    []SlackMpim //!< multiparty IM
	Ims      []SlackIm
	Bots     []SlackBot
}

//==============================
// internal settings
//==============================

var g_IgnoreMessageTypes = map[string]struct{}{
	"channel_marked":   struct{}{},
	"file_change":      struct{}{},
	"file_public":      struct{}{},
	"file_shared":      struct{}{},
	"group_marked":     struct{}{},
	"im_marked":        struct{}{},
	"message":          struct{}{},
	"perf_change":      struct{}{},
	"reaction_added":   struct{}{},
	"reaction_removed": struct{}{},
	"thread_marked":    struct{}{},
	"user_change":      struct{}{},
	"user_typing":      struct{}{},
}

//==============================
// global variables
//==============================

//! maps user-id, channel-id, etc and name
var g_IdNameMap map[string]string

var g_LastUser = ""
var g_LastChannel = ""
var g_LastThreadTs = time.Unix(0, 0)

var g_MentionPattern = regexp.MustCompile(`<@([^>|]+)(\|([^>]+))?>`)
var g_ChannelPattern = regexp.MustCompile(`<#([^>|]+)(\|([^>]+))?>`)
var g_KeywordPattern = regexp.MustCompile(`<!([^>|]+)(\|([^>]+))?>`)
var g_NotificationPatterns []*regexp.Regexp

var g_Config Config

//==============================
// entry point
//==============================

func main() {
	console.Initialize()
	defer console.Finalize()

	err := loadConfig("config.toml")
	if err != nil {
		log.Fatal(err)
		return
	}

	fmt.Println("Connecting...")
	waitNS := 1 * time.Second

	var lastError error

	for {
		ws, err := connect(g_Config.General.Token)
		if err != nil {
			goto L_Error
		}
		defer ws.Close()

		waitNS = 1 * time.Second
		lastError = nil

		err = receiveRoutine(ws)
		if err != nil {
			ws.Close()
			goto L_Error
		}

	L_Error:

		if !errorEquals(err, lastError) {
			log.Print(err)
			log.Printf("Connecting...\n", waitNS/time.Second)
			lastError = err
		}

		time.Sleep(waitNS)
		waitNS = waitNS * 2
		if waitNS > 15*time.Second {
			waitNS = 15 * time.Second
		}
	}
}

func errorEquals(a error, b error) bool {
	if a != nil && b != nil {
		return a.Error() == b.Error()
	}
	return a == b
}

func loadConfig(path string) error {
	_, err := toml.DecodeFile(path, &g_Config)
	if err != nil {
		return err
	}

	if g_Config.Notification.Patterns != nil {
		for _, pattern := range g_Config.Notification.Patterns {
			if regex, err := regexp.Compile(pattern); err != nil {
				log.Print(err)
			} else {
				g_NotificationPatterns = append(g_NotificationPatterns, regex)
			}
		}
	}

	return nil
}

//! login to Slack and connect websocket
func connect(token string) (*websocket.Conn, error) {
	session, err := login(token)
	if err != nil {
		return nil, err
	}

	g_IdNameMap = generateIdNameMap(session)

	ws, err := websocket.Dial(session.Url, "", "http://localhost/")
	if err != nil {
		return nil, err
	}

	return ws, nil
}

//! login to Slack
func login(token string) (SlackSession, error) {
	query := url.Values{}
	query.Set("token", token)

	request, err := http.NewRequest(
		"POST",
		"https://slack.com/api/rtm.start",
		strings.NewReader(query.Encode()),
	)
	if err != nil {
		return SlackSession{}, err
	}

	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		return SlackSession{}, err
	}
	defer response.Body.Close()

	data, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return SlackSession{}, err
	}

	session := SlackSession{}
	if err := json.Unmarshal(data, &session); err != nil {
		return SlackSession{}, err
	}

	return session, nil
}

//! generate mapping to id and name from SlackSession
func generateIdNameMap(session SlackSession) map[string]string {
	result := map[string]string{}

	for _, user := range session.Users {
		result[user.Id] = user.Name
	}
	for _, bot := range session.Bots {
		result[bot.Id] = bot.Name
	}
	for _, channel := range session.Channels {
		result[channel.Id] = channel.Name
	}
	for _, group := range session.Groups {
		result[group.Id] = group.Name
	}
	for _, mpim := range session.Mpims {
		result[mpim.Id] = mpim.Name
	}
	for _, im := range session.Ims {
		result[im.Id] = result[im.UserId]
	}

	return result
}

//! receiving loop
func receiveRoutine(ws *websocket.Conn) error {
	for {
		// receive from ws, and map to string and interface{} from JSON
		var unmappedMsg interface{}

		if err := websocket.JSON.Receive(ws, &unmappedMsg); err != nil {
			return err
		}

		msg := unmappedMsg.(map[string]interface{})

		// debug log
		if _, exist := g_IgnoreMessageTypes[msg["type"].(string)]; !exist {
			//log.Printf("type: %s, subtype: %s\n", msg["type"], msg["subtype"])
		}

		// dispatch from type
		switch msg["type"] {
		case "hello":
			fmt.Println("Connected!")
		case "bot_added":
			onBotAdded(msg)
		case "channel_created":
			onChannelCreated(msg)
		case "channel_joined":
			onChannelJoined(msg)
		case "group_joined":
			onGroupJoined(msg)
		case "message":
			onMessage(msg)
		case "team_join":
			onTeamJoin(msg)
		case "user_change":
			onUserChange(msg)
		}
	}

	return nil
}

//==============================
// type: "bot_added"
//==============================

func onBotAdded(msg map[string]interface{}) {
	id := msg["bot"].(map[string]interface{})["id"].(string)
	name := msg["bot"].(map[string]interface{})["name"].(string)
	g_IdNameMap[id] = name
}

//==============================
// type: "channel_created"
//==============================
func onChannelCreated(msg map[string]interface{}) {
	id := msg["channel"].(map[string]interface{})["id"].(string)
	name := msg["channel"].(map[string]interface{})["name"].(string)
	g_IdNameMap[id] = name
}

//==============================
// type: "channel_joined"
//==============================
func onChannelJoined(msg map[string]interface{}) {
}

//==============================
// type: "group_joined"
//==============================

func onGroupJoined(msg map[string]interface{}) {
}

//==============================
// type: "message"
//==============================

func onMessage(msg map[string]interface{}) {
	switch msg["subtype"] {
	case nil:
		onPureMessage(msg)
	case "bot_message":
		onMessageBot(msg)
	case "file_comment":
		onMessageFileComment(msg)
	case "file_mention":
		return
	case "file_share":
		onMessageFileShare(msg)
	case "me_message":
		onMessageMe(msg)
	case "message_changed":
		onMessageChanged(msg)
	case "message_replied":
		onMessageReplied(msg)
	}
}

func onPureMessage(msg map[string]interface{}) {
	timestamp := getTimestamp(msg)
	threadTs := getThreadTs(msg)
	channel := getChannel(msg)
	userType := getUserType(msg)
	user := getUser(msg)
	text := msg["text"].(string)

	printMessage(timestamp, threadTs, channel, userType, user, text, "")
}

func onMessageBot(msg map[string]interface{}) {
	timestamp := getTimestamp(msg)
	threadTs := getThreadTs(msg)
	channel := getChannel(msg)
	userType := getUserType(msg)
	user := getBot(msg)
	text := getText(msg)
	toRemoveLastUser := false

	if attachments, exist := msg["attachments"].([]interface{}); exist {
		if attachment, exist := attachments[0].(map[string]interface{}); exist {
			title := ""
			text, title = getAttachmentText(attachment)
			text = title + text
			toRemoveLastUser = true
		}
	}

	printMessage(timestamp, threadTs, channel, userType, user, text, "")

	if toRemoveLastUser {
		// display header on next message
		g_LastUser = ""
	}
}

func onMessageFileComment(msg map[string]interface{}) {
	file, exist := msg["file"].(map[string]interface{})
	if !exist {
		return
	}
	comment, exist := msg["comment"].(map[string]interface{})
	if !exist {
		return
	}
	timestamp := getTimestamp(msg)
	threadTs := getThreadTs(msg)
	channel := getChannel(msg)
	userType := getUserType(msg)
	user := getUser(comment)
	title := "comment to: " + getTitle(file)
	text := comment["comment"].(string)

	title = "\033[44m" + strings.TrimSpace(title) + "\033[0m\n"
	text = title + text

	printMessage(timestamp, threadTs, channel, userType, user, text, "")

	// display header on next message
	g_LastUser = ""
}

func onMessageFileShare(msg map[string]interface{}) {
	var text = ""

	file, exist := msg["file"].(map[string]interface{})
	if !exist {
		return
	}
	timestamp := getTimestamp(msg)
	threadTs := getThreadTs(msg)
	channel := getChannel(msg)
	userType := getUserType(msg)
	user := getUser(msg)
	title := "file: " + getTitle(file)
	if preview, exist := file["preview"].(string); exist {
		if isPreviewTruncated(file) {
			preview = preview + "..."
		}
		title = "\033[44m" + strings.TrimSpace(title) + "\033[0m\n"
		text = title + preview
	} else {
		text = msg["text"].(string)
	}

	printMessage(timestamp, threadTs, channel, userType, user, text, "")

	// display header on next message
	g_LastUser = ""
}

func onMessageMe(msg map[string]interface{}) {
	timestamp := getTimestamp(msg)
	threadTs := getThreadTs(msg)
	channel := getChannel(msg)
	userType := getUserType(msg)
	user := getUser(msg)
	text := "\033[3m\033[90m" + msg["text"].(string) + "\033[0m"

	printMessage(timestamp, threadTs, channel, userType, user, text, "")
}

func onMessageChanged(msg map[string]interface{}) {
	message, exist := msg["message"].(map[string]interface{})
	if !exist {
		return
	}
	prevMessage, exist := msg["previous_message"].(map[string]interface{})
	if !exist {
		return
	}
	timestamp := getTimestamp(message)
	threadTs := getThreadTs(msg)
	channel := getChannel(msg)
	userType := getUserType(msg)
	user := getUser(message)
	text := getText(message)
	prevText := getText(prevMessage)
	if text != prevText {
		annotation := " \033[93m(edited)\033[0m"
		printMessage(timestamp, threadTs, channel, userType, user, text, annotation)
	}

	attText, attTitle := getAttachmentsText(message)
	attText = attTitle + attText
	prevAttText, prevAttTitle := getAttachmentsText(prevMessage)
	prevAttText = prevAttTitle + prevAttText
	if attText != prevAttText {
		printMessage(timestamp, threadTs, channel, userType, user, attText, "")

		// display header on next message
		g_LastUser = ""
	}
}

func onMessageReplied(msg map[string]interface{}) {
	message, exist := msg["message"].(map[string]interface{})
	if !exist {
		return
	}
	timestamp := getTimestamp(message)
	threadTs := getThreadTs(message)
	channel := getChannel(msg)
	userType := getUserType(message)
	user := getUser(message)
	text := getText(message)

	printMessage(timestamp, threadTs, channel, userType, user, text, "")
}

func getChannel(msg map[string]interface{}) string {
	if mayChannel, exist := msg["channel"]; exist {
		return g_IdNameMap[mayChannel.(string)]
	}
	return ""
}

func getUserType(msg map[string]interface{}) string {
	userType := ""
	if _, exist := msg["bot_id"]; exist {
		userType = userType + "[bot]"
	}
	if _, exist := msg["app_id"]; exist {
		userType = userType + "[app]"
	}
	return userType
}

func getUser(msg map[string]interface{}) string {
	if mayUser, exist := msg["user"]; exist {
		return g_IdNameMap[mayUser.(string)]
	}
	return ""
}

func getBot(msg map[string]interface{}) string {
	if mayBot, exist := msg["bot_id"]; exist {
		return g_IdNameMap[mayBot.(string)]
	}
	return ""
}

func getText(msg map[string]interface{}) string {
	if mayText, exist := msg["text"]; exist {
		return mayText.(string)
	}
	return ""
}

func getTimestamp(msg map[string]interface{}) time.Time {
	fTs := 0.0
	if strTs, exist := msg["ts"]; exist {
		fTs, _ = strconv.ParseFloat(strTs.(string), 64)
	}
	return time.Unix(int64(fTs), 0)
}

func getThreadTs(msg map[string]interface{}) time.Time {
	fTs := 0.0
	if strTs, exist := msg["thread_ts"]; exist {
		fTs, _ = strconv.ParseFloat(strTs.(string), 64)
	}
	return time.Unix(int64(fTs), 0)
}

func getTitle(msg map[string]interface{}) string {
	if title, exist := msg["title"]; exist {
		return title.(string)
	}
	return ""
}

func getPreview(msg map[string]interface{}) string {
	if preview, exist := msg["preview"]; exist {
		return preview.(string)
	}
	return ""
}

func isPreviewTruncated(msg map[string]interface{}) bool {
	if isTruncated, exist := msg["preview_is_truncated"]; exist {
		return isTruncated.(bool)
	}
	return false
}

func getAttachmentsText(msg map[string]interface{}) (string, string) {
	if attachments, exist := msg["attachments"].([]interface{}); exist {
		if attachment, exist := attachments[0].(map[string]interface{}); exist {
			return getAttachmentText(attachment)
		}
	}
	return "", ""
}

func getAttachmentText(attachment map[string]interface{}) (string, string) {
	text := ""
	title := ""
	exist := false

	if serviceName, exist := attachment["service_name"].(string); exist {
		title = title + serviceName + ": "
	}
	if authorName, exist := attachment["author_name"].(string); exist {
		title = title + authorName + " "
	}
	if title, exist := attachment["title"].(string); exist {
		title = title + title + " "
	}
	if footer, exist := attachment["footer"].(string); exist {
		title = title + " (" + footer + ") "
	}
	if len(title) > 0 {
		title = "\033[44m" + strings.TrimSpace(title) + "\033[0m\n"
	}
	if text, exist = attachment["text"].(string); !exist {
		text, _ = attachment["fallback"].(string)
	}
	if textLen := len(text); textLen > 1000 {
		text = text[:1000] + "..."
	}

	return text, title
}


func printMessage(
	timestamp time.Time,
	threadTs time.Time,
	channel string,
	userType string,
	user string,
	text string,
	annotation string,
) {
	if equalsAnyKeywords(channel, g_Config.Notification.MuteChannels) {
		return
	}
	if equalsAnyKeywords(user, g_Config.Notification.MuteUsers) {
		return
	}
	if len(text) == 0 {
		return
	}

	strTimestamp := timestamp.Format("2006/01/02 15:04:05")
	if threadTs.Unix() != 0 {
		strTimestamp = strTimestamp + " [at " + threadTs.Format("2006/01/02 15:04:05") + "]"
	}

	if channel != g_LastChannel {
		// insert a empty line and header
		fmt.Printf(
			"\n\033[93m@%-18s #%-20s %s\033[0m\n",
			userType+user,
			channel,
			strTimestamp,
		)
	} else if user != g_LastUser || !threadTs.Equal(g_LastThreadTs) {
		// display header
		fmt.Printf(
			"\033[93m@%-18s #%-20s %s\033[0m\n",
			userType+user,
			channel,
			strTimestamp,
		)
	}

	text = unescape(text)
	if matchAnyPatterns(text, g_NotificationPatterns) {
		text = "\033[5;95m" + text + "\033[0m"
	}

	// display body
	fmt.Printf("%s%s\n", text, annotation)

	g_LastChannel = channel
	g_LastUser = user
	g_LastThreadTs = threadTs
}

func unescape(text string) string {
	text = g_ChannelPattern.ReplaceAllString(text, "#$3")
	text = g_KeywordPattern.ReplaceAllString(text, "@$2")

	isMatching := true
	for isMatching {
		isMatching = false
		if index := g_MentionPattern.FindStringSubmatchIndex(text); index != nil {
			isMatching = true
			text = text[:index[0]] + "@" + g_IdNameMap[text[index[2]:index[3]]] + text[index[1]:]
		}
	}
	return html.UnescapeString(text)
}

func matchAnyPatterns(text string, patterns []*regexp.Regexp) bool {
	for _, pattern := range patterns {
		if pattern.MatchString(text) {
			return true
		}
	}
	return false
}

func equalsAnyKeywords(text string, keywords []string) bool {
	for _, keyword := range keywords {
		if text == keyword {
			return true
		}
	}
	return false
}

//==============================
// type: "team_join"
//==============================

func onTeamJoin(msg map[string]interface{}) {
	id := msg["user"].(map[string]interface{})["id"].(string)
	name := msg["user"].(map[string]interface{})["name"].(string)
	g_IdNameMap[id] = name
}

//==============================
// type: "user_change"
//==============================

func onUserChange(msg map[string]interface{}) {
	id := msg["user"].(map[string]interface{})["id"].(string)
	name := msg["user"].(map[string]interface{})["name"].(string)
	g_IdNameMap[id] = name
}
