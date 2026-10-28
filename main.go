package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
)

type Server struct {
	Host        string   `json:"host"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

// gwkit path file
var (
	baseDir            = exeDir()
	kinitPassword      = filepath.Join(baseDir, ".kinit_passwd")
	serverListJSONFile = filepath.Join(baseDir, "server_list.json")
)

var (
	users        = []string{"irteam", "irteamsu"}
	loginMethods = []string{"rlogin", "ssh"}
	helpLines    = []string{
		"[/]: change user to rlogin, [,]: change rlogin/ssh",
		"[ctrl-n]: register new server     [ctrl-x]: delete server",
		"[ctrl-e]: modify server           [ctrl-c]: quit or close popup window",
		"[ctrl-l]: load old gw file        [ctrl-r]: reset popup input",
		"- registered server will be saved when terminated. (server_list.json)",
		"- make \"~/.kinit_passwd\" to execute kinit automatically.",
		"- enter a keyword to filter the list.",
		"- search for hosts, tags, and descriptions using case-insensitive keywords.",
	}
)

const (
	helpRows = 10
	topRows  = 3
	padLen   = 5
)

var (
	stDefault  = tcell.StyleDefault
	stSelected = stDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorYellow)
	stKeyword  = stDefault.Foreground(tcell.ColorRed)
	stSelKey   = stDefault.Foreground(tcell.ColorRed).Background(tcell.ColorYellow)
	stPopup    = stDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite)
	stError    = stPopup.Foreground(tcell.ColorRed)
)

func exeDir() string {
	p, err := os.Executable()
	if err == nil {
		p, err = filepath.EvalSymlinks(p)
	}
	if err != nil {
		return "."
	}
	return filepath.Dir(p)
}

// lower keeps the rune count, so match positions map back onto the original text.
func lower(s string) string { return strings.Map(unicode.ToLower, s) }

func matches(s *Server, kw string) bool {
	if strings.Contains(lower(s.Host), kw) || strings.Contains(lower(s.Description), kw) {
		return true
	}
	for _, t := range s.Tags {
		if strings.Contains(lower(t), kw) {
			return true
		}
	}
	return false
}

// highlight marks the runes of text that are part of any keyword match.
func highlight(text string, kws []string) []bool {
	t := []rune(lower(text))
	mask := make([]bool, len(t))
	for _, k := range kws {
		kr := []rune(k)
		for i := 0; i+len(kr) <= len(t); i++ {
			if string(t[i:i+len(kr)]) == k {
				for j := range kr {
					mask[i+j] = true
				}
			}
		}
	}
	return mask
}

// lineInput is a single line text field with a cursor.
type lineInput struct {
	value  []rune
	cursor int
}

func newLineInput(v string) *lineInput {
	r := []rune(v)
	return &lineInput{r, len(r)}
}

func (in *lineInput) String() string { return string(in.value) }

func (in *lineInput) handle(ev *tcell.EventKey) {
	switch ev.Key() {
	case tcell.KeyLeft:
		if in.cursor > 0 {
			in.cursor--
		}
	case tcell.KeyRight:
		if in.cursor < len(in.value) {
			in.cursor++
		}
	case tcell.KeyHome, tcell.KeyCtrlA:
		in.cursor = 0
	case tcell.KeyEnd, tcell.KeyCtrlE:
		in.cursor = len(in.value)
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if in.cursor > 0 {
			in.value = slices.Delete(in.value, in.cursor-1, in.cursor)
			in.cursor--
		}
	case tcell.KeyDelete:
		if in.cursor < len(in.value) {
			in.value = slices.Delete(in.value, in.cursor, in.cursor+1)
		}
	case tcell.KeyCtrlR:
		in.value, in.cursor = nil, 0
	case tcell.KeyRune:
		in.value = slices.Insert(in.value, in.cursor, ev.Rune())
		in.cursor++
	}
}

// draw prints the field at (x, y) and returns the screen x of the cursor.
func (in *lineInput) draw(s tcell.Screen, x, y, maxX int, st tcell.Style) int {
	drawStr(s, x, y, maxX, in.String(), st)
	return x + runewidth.StringWidth(string(in.value[:in.cursor]))
}

func drawStr(s tcell.Screen, x, y, maxX int, str string, st tcell.Style) int {
	for _, r := range str {
		w := runewidth.RuneWidth(r)
		if x+w > maxX {
			break
		}
		s.SetContent(x, y, r, nil, st)
		x += w
	}
	return x
}

func box(s tcell.Screen, x, y, w, h int, title string, st tcell.Style) {
	for j := y; j < y+h; j++ {
		for i := x; i < x+w; i++ {
			s.SetContent(i, j, ' ', nil, st)
		}
	}
	for i := x + 1; i < x+w-1; i++ {
		s.SetContent(i, y, tcell.RuneHLine, nil, st)
		s.SetContent(i, y+h-1, tcell.RuneHLine, nil, st)
	}
	for j := y + 1; j < y+h-1; j++ {
		s.SetContent(x, j, tcell.RuneVLine, nil, st)
		s.SetContent(x+w-1, j, tcell.RuneVLine, nil, st)
	}
	s.SetContent(x, y, tcell.RuneULCorner, nil, st)
	s.SetContent(x+w-1, y, tcell.RuneURCorner, nil, st)
	s.SetContent(x, y+h-1, tcell.RuneLLCorner, nil, st)
	s.SetContent(x+w-1, y+h-1, tcell.RuneLRCorner, nil, st)
	drawStr(s, x+5, y, x+w-1, title, st)
}

type App struct {
	s         tcell.Screen
	servers   []*Server
	filtered  []*Server
	keyword   *lineInput
	userIdx   int
	methodIdx int
	selected  int
	top       int
}

func (a *App) keywords() []string { return strings.Fields(lower(a.keyword.String())) }

func (a *App) listRows() int {
	_, h := a.s.Size()
	return h - helpRows - topRows - 3
}

func (a *App) filter(selected int) {
	kws := a.keywords()
	a.filtered = nil
	for _, s := range a.servers {
		if !slices.ContainsFunc(kws, func(k string) bool { return !matches(s, k) }) {
			a.filtered = append(a.filtered, s)
		}
	}
	a.top = 0
	if selected > len(a.filtered)-1 {
		selected = -1
	}
	a.selected = selected
}

func (a *App) move(delta int) {
	a.selected = min(max(a.selected+delta, 0), len(a.filtered)-1)
	if a.selected < a.top {
		a.top = max(a.selected, 0)
	}
	if n := a.listRows(); a.selected >= a.top+n {
		a.top = a.selected - n + 1
	}
}

func (a *App) current() *Server {
	if a.selected < 0 {
		return nil
	}
	return a.filtered[a.selected]
}

func (a *App) draw() {
	s := a.s
	s.Clear()
	w, h := s.Size()
	half := w / 2

	box(s, 0, 0, w, helpRows, "Help", stDefault)
	for i, l := range helpLines {
		drawStr(s, 2, 1+i, w-1, l, stDefault)
	}

	box(s, 0, helpRows, half, topRows, "", stDefault)
	drawStr(s, 2, helpRows+1, half-1, fmt.Sprintf("user : %s, [ %s ]", users[a.userIdx], loginMethods[a.methodIdx]), stDefault)

	box(s, half, helpRows, w-half, topRows, "", stDefault)
	x := drawStr(s, half+2, helpRows+1, w-1, "keyword : ", stDefault)
	s.ShowCursor(a.keyword.draw(s, x, helpRows+1, w-1, stDefault), helpRows+1)

	y0 := helpRows + topRows
	box(s, 0, y0, w, h-y0, "", stDefault)
	maxHost, maxTags := 30, 30
	if len(a.servers) > 0 {
		maxHost, maxTags = 0, 0
		for _, srv := range a.servers {
			maxHost = max(maxHost, runewidth.StringWidth(srv.Host))
			maxTags = max(maxTags, runewidth.StringWidth(strings.Join(srv.Tags, ", ")))
		}
	}
	hostX, tagsX, descX := padLen, maxHost+padLen*2, maxHost+maxTags+padLen*3
	drawStr(s, hostX, y0, w-1, "Host", stDefault)
	drawStr(s, tagsX, y0, w-1, "Tags", stDefault)
	drawStr(s, descX, y0, w-1, "Description", stDefault)

	kws := a.keywords()
	for i := a.top; i < len(a.filtered) && i < a.top+a.listRows(); i++ {
		srv := a.filtered[i]
		y := y0 + 2 + i - a.top
		normal, hl := stDefault, stKeyword
		if i == a.selected {
			normal, hl = stSelected, stSelKey
			drawStr(s, 1, y, w-1, strings.Repeat(" ", w), normal)
		}
		for _, col := range []struct {
			x    int
			text string
		}{{hostX, srv.Host}, {tagsX, strings.Join(srv.Tags, ", ")}, {descX, srv.Description}} {
			mask := highlight(col.text, kws)
			cx := col.x
			for j, r := range []rune(col.text) {
				st := normal
				if mask[j] {
					st = hl
				}
				cx = drawStr(s, cx, y, w-1, string(r), st)
			}
		}
	}
}

// form shows a modal popup and returns the edited values, or nil when cancelled with ctrl-c.
func (a *App) form(title string, labels, values []string, validate func([]string) string) []string {
	s := a.s
	ins := make([]*lineInput, len(labels))
	for i := range labels {
		ins[i] = newLineInput(values[i])
	}
	idx, errMsg := 0, ""
	for {
		a.draw()
		w, _ := s.Size()
		px, py, pw := max(w/2-50, 0), helpRows+topRows+4, 100
		box(s, px, py, pw, len(labels)*2+3, title, stPopup)
		for i, l := range labels {
			fy := py + 2 + i*2
			cx := ins[i].draw(s, drawStr(s, px+2, fy, px+pw-1, l+" ", stPopup), fy, px+pw-1, stPopup)
			if i == idx {
				s.ShowCursor(cx, fy)
			}
		}
		drawStr(s, px+2, py+3, px+pw-1, errMsg, stError)
		s.Show()

		ev, ok := s.PollEvent().(*tcell.EventKey)
		if !ok {
			s.Sync()
			continue
		}
		errMsg = ""
		switch ev.Key() {
		case tcell.KeyCtrlC:
			return nil
		case tcell.KeyUp:
			idx = (idx + len(ins) - 1) % len(ins)
		case tcell.KeyDown:
			idx = (idx + 1) % len(ins)
		case tcell.KeyEnter:
			vals := make([]string, len(ins))
			for i, in := range ins {
				vals[i] = in.String()
			}
			if validate != nil {
				errMsg = validate(vals)
			}
			if errMsg == "" {
				return vals
			}
		default:
			ins[idx].handle(ev)
		}
	}
}

func (a *App) insert(srv *Server) {
	if slices.ContainsFunc(a.servers, func(s *Server) bool { return s.Host == srv.Host }) {
		return
	}
	a.servers = append([]*Server{srv}, a.servers...)
	a.filter(-1)
}

// editServer registers a new server when cur is nil, otherwise modifies cur.
func (a *App) editServer(cur *Server) {
	title, vals := "Register", []string{"", "", ""}
	if cur != nil {
		title, vals = "Modify", []string{cur.Host, cur.Description, strings.Join(cur.Tags, " ")}
	}
	res := a.form(title, []string{"Host :", "Description :", "Tags :"}, vals, func(v []string) string {
		if cur != nil && cur.Host == v[0] {
			return ""
		}
		if slices.ContainsFunc(a.servers, func(s *Server) bool { return s.Host == v[0] }) {
			return "Duplicated Host !!!"
		}
		return ""
	})
	if res == nil {
		return
	}
	srv := &Server{res[0], res[1], strings.FieldsFunc(res[2], func(r rune) bool { return r == ',' || r == ' ' })}
	if cur != nil {
		*cur = *srv
	} else {
		a.insert(srv)
	}
}

func (a *App) deleteCurrent() {
	cur := a.current()
	if cur == nil {
		return
	}
	a.servers = slices.DeleteFunc(a.servers, func(s *Server) bool { return s.Host == cur.Host })
	a.filter(a.selected)
}

func (a *App) loadOldGwFile() {
	home, _ := os.UserHomeDir()
	res := a.form("Load old gateway .known_hosts (.known_host can be omitted)", []string{"Path :"}, []string{home}, nil)
	if res == nil {
		return
	}
	p := res[0]
	if !strings.Contains(p, ".known_hosts") {
		p = filepath.Join(p, ".known_hosts")
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		if f := strings.Fields(line); len(f) > 0 {
			a.insert(&Server{f[0], strings.Join(f[1:], " "), []string{}})
		}
	}
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func (a *App) connect() {
	cur := a.current()
	if cur == nil {
		return
	}
	user := users[a.userIdx]
	a.s.Suspend()
	defer a.s.Resume()
	if loginMethods[a.methodIdx] == "rlogin" && run("rlogin", "-l", user, cur.Host) == nil {
		return
	}
	run("ssh", user+"@"+cur.Host)
}

func (a *App) loop() {
	for {
		a.draw()
		a.s.Show()
		ev, ok := a.s.PollEvent().(*tcell.EventKey)
		if !ok {
			a.s.Sync()
			continue
		}
		switch ev.Key() {
		case tcell.KeyCtrlC:
			return
		case tcell.KeyUp:
			a.move(-1)
		case tcell.KeyDown:
			a.move(1)
		case tcell.KeyPgUp:
			a.move(-20)
		case tcell.KeyPgDn:
			a.move(20)
		case tcell.KeyEnter:
			a.connect()
		case tcell.KeyCtrlX:
			a.deleteCurrent()
		case tcell.KeyCtrlE:
			if cur := a.current(); cur != nil {
				a.editServer(cur)
			}
		case tcell.KeyCtrlN:
			a.editServer(nil)
		case tcell.KeyCtrlL:
			a.loadOldGwFile()
		default:
			switch {
			case ev.Key() == tcell.KeyRune && ev.Rune() == '/':
				a.userIdx = (a.userIdx + 1) % len(users)
			case ev.Key() == tcell.KeyRune && (ev.Rune() == ',' || ev.Rune() == '\\'):
				a.methodIdx = (a.methodIdx + 1) % len(loginMethods)
			default:
				a.keyword.handle(ev)
				a.filter(-1)
			}
		}
	}
}

func loadServers() ([]*Server, error) {
	var servers []*Server
	data, err := os.ReadFile(serverListJSONFile)
	if os.IsNotExist(err) {
		return servers, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &servers); err != nil {
		return nil, fmt.Errorf("%s: %w", serverListJSONFile, err)
	}
	sort.Slice(servers, func(i, j int) bool { return servers[i].Host < servers[j].Host })
	return servers, nil
}

func saveServers(servers []*Server) error {
	data, err := json.Marshal(append([]*Server{}, servers...))
	if err != nil {
		return err
	}
	return os.WriteFile(serverListJSONFile, data, 0644)
}

func printProgress(i, total int, prefix, suffix string, barLength int) {
	filled := barLength * i / total
	bar := strings.Repeat("\033[94m▇\033[0m", filled) + strings.Repeat("-", barLength-filled)
	fmt.Printf("\r%s |%s| %.1f%% %s", prefix, bar, 100*float64(i)/float64(total), suffix)
	if i == total {
		fmt.Println()
	}
}

func decodeJSON(resp *http.Response, err error, v any) error {
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(v)
}

// initServerList fetches "my server groups" from tips and writes them to server_list.json.
func initServerList() error {
	fmt.Print("\033[94m" + banner + "\033[0m")
	fmt.Print("SSO ID : ")
	id, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	fmt.Print("SSO PW : ")
	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return err
	}

	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}
	fmt.Println("first page in tips.nhnent.com")
	resp, err := c.Get("https://tips.nhnent.com")
	if err != nil {
		return err
	}
	var page bytes.Buffer
	page.ReadFrom(resp.Body)
	resp.Body.Close()
	m := regexp.MustCompile(`action="([^"]*)"`).FindStringSubmatch(page.String())
	if m == nil {
		return fmt.Errorf("login form not found in tips.nhnent.com")
	}
	loginURL := html.UnescapeString(m[1])

	fmt.Println("login... " + loginURL)
	resp, err = c.PostForm(loginURL, url.Values{
		"x":        {"53"},
		"y":        {"48"},
		"language": {"ko_KR"},
		"times":    {"Asia/Seoul:+9"},
		"username": {strings.TrimSpace(id)},
		"password": {string(pw)},
	})
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Your sso could not be authentication.")
	}

	fmt.Println("Fetching your server list....")
	body, _ := json.Marshal(map[string]any{"searchCategory": "serviceOrPlatform", "searchText": "", "orderType": 1, "orderFieldName": "service_name", "all": "true"})
	var groups struct {
		ServerGroupForManagementData struct {
			Data []struct {
				ServerGroupCode json.RawMessage `json:"serverGroupCode"`
				ServiceName     string          `json:"serviceName"`
			} `json:"data"`
		} `json:"serverGroupForManagementData"`
	}
	resp, err = c.Post("https://tips.nhnent.com/config/serverGroups/my/retrieve", "application/json", bytes.NewReader(body))
	if err := decodeJSON(resp, err, &groups); err != nil {
		return err
	}

	result := []*Server{}
	list := groups.ServerGroupForManagementData.Data
	for i, g := range list {
		var group struct {
			ServerGroupForManagement struct {
				Data struct {
					ServerList []struct {
						HostName string `json:"hostName"`
						Tags     string `json:"tags"`
					} `json:"serverList"`
				} `json:"data"`
			} `json:"serverGroupForManagement"`
		}
		code := strings.Trim(string(g.ServerGroupCode), `"`)
		resp, err := c.Get("https://tips.nhnent.com/config/serverGroups/management/" + code + "?formMode=1&pageNum=1&orderFieldName=host_name&orderType=1")
		if err := decodeJSON(resp, err, &group); err != nil {
			return err
		}
		for _, srv := range group.ServerGroupForManagement.Data.ServerList {
			result = append(result, &Server{srv.HostName, g.ServiceName, strings.Fields(srv.Tags)})
		}
		printProgress(i+1, len(list), "Fetch Progress:", "Complete", 50)
	}
	return saveServers(result)
}

func kinit() {
	cmd := exec.Command("kinit")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if f, err := os.Open(kinitPassword); err == nil {
		defer f.Close()
		cmd.Stdin = f
	}
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "kinit:", err)
	}
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "init" {
		if err := initServerList(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	// stop before the TUI so a broken file is never overwritten on quit
	servers, err := loadServers()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	kinit()

	s, err := tcell.NewScreen()
	if err == nil {
		err = s.Init()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	a := &App{s: s, servers: servers, keyword: newLineInput(""), methodIdx: 1}
	a.filter(-1)
	a.loop()
	s.Fini()

	if err := saveServers(a.servers); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("Goodbye :)")
}

const banner = `
 __    __  __    __  __    __         ______    ______    ______
|  \  |  \|  \  |  \|  \  |  \       /      \  /      \  /      \
| $$\ | $$| $$  | $$| $$\ | $$      |  $$$$$$\|  $$$$$$\|  $$$$$$\
| $$$\| $$| $$__| $$| $$$\| $$      | $$___\$$| $$___\$$| $$  | $$
| $$$$\ $$| $$    $$| $$$$\ $$       \$$    \  \$$    \ | $$  | $$
| $$\$$ $$| $$$$$$$$| $$\$$ $$       _\$$$$$$\ _\$$$$$$\| $$  | $$
| $$ \$$$$| $$  | $$| $$ \$$$$      |  \__| $$|  \__| $$| $$__/ $$
| $$  \$$$| $$  | $$| $$  \$$$       \$$    $$ \$$    $$ \$$    $$
 \$$   \$$ \$$   \$$ \$$   \$$        \$$$$$$   \$$$$$$   \$$$$$$
`
