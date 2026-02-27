package main

// Shinefetch 󰄳
import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unsafe"

	"github.com/mattn/go-runewidth"
	"github.com/tailscale/hujson"
)

// ──────────────── Constants & Types ────────────────

type Config struct {
	ShinyChance   int    `json:"shiny_chance"`    // 1 in X chance for shiny
	BoxStyle      string `json:"box_style"`       // rounded, sharp, double, heavy
	Gap           int    `json:"gap"`             // space between pokemon and box
	Animation     bool   `json:"animation"`       // always animate border if true
	TrainerName   string `json:"trainer_name"`    // override user name
	Align         string `json:"align"`           // center or left
	PrintAndExit  bool   `json:"print_and_exit"`  // print once and quit (no interactive)
	ShinyBoxStyle string `json:"shiny_box_style"` // border style for shiny pokemon
}

func loadConfig() Config {
	c := Config{
		ShinyChance:   20,
		BoxStyle:      "rounded",
		Gap:           8,
		Animation:     true,
		TrainerName:   "",
		Align:         "center",
		PrintAndExit:  false,
		ShinyBoxStyle: "double",
	}
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".config", "shinefetch", "settings.jsonc")
	if data, err := os.ReadFile(path); err == nil {
		if std, err := hujson.Standardize(data); err == nil {
			json.Unmarshal(std, &c)
		}
	} else {
		path = filepath.Join(home, ".config", "shinefetch", "settings.json")
		if data, err := os.ReadFile(path); err == nil {
			json.Unmarshal(data, &c)
		}
	}
	return c
}

var typeColors = map[string]string{
	"normal":   "168;167;122",
	"fire":     "238;129;48",
	"water":    "99;144;240",
	"electric": "247;208;44",
	"grass":    "122;199;76",
	"ice":      "150;217;214",
	"fighting": "194;46;40",
	"poison":   "163;62;161",
	"ground":   "226;191;101",
	"flying":   "169;143;243",
	"psychic":  "249;85;135",
	"bug":      "166;185;26",
	"rock":     "182;161;54",
	"ghost":    "115;87;151",
	"dragon":   "111;53;252",
	"dark":     "112;87;70",
	"steel":    "183;183;206",
	"fairy":    "214;133;173",
}

type BoxStyle struct {
	TL, TR, BL, BR string
	H, V           string
	LT, RT         string
}

var boxStyles = map[string]BoxStyle{
	"rounded": {"╭", "╮", "╰", "╯", "─", "│", "├", "┤"},
	"sharp":   {"┌", "┐", "└", "┘", "─", "│", "├", "┤"},
	"double":  {"╔", "╗", "╚", "╝", "═", "║", "╠", "╣"},
	"heavy":   {"┏", "┓", "┗", "┛", "━", "┃", "┣", "┫"},
}

type Row struct {
	K, V  string
	IsSep bool
	IsRaw bool
}

type Stats struct {
	ShinyCount int `json:"shiny_count"`
}

func loadStats() Stats {
	// Priority: Config folder
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".config", "shinefetch", "stats.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return Stats{}
	}
	var s Stats
	json.Unmarshal(data, &s)
	return s
}

func saveStats(s Stats) {
	data, _ := json.Marshal(s)

	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".config", "shinefetch")
	os.MkdirAll(dir, 0755)
	path := filepath.Join(dir, "stats.json")
	os.WriteFile(path, data, 0644)
}

// ──────────────── Helpers ────────────────

func stripAnsi(s string) string {
	re := regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)
	return re.ReplaceAllString(s, "")
}

func getVisibleLen(s string) int {
	return runewidth.StringWidth(stripAnsi(s))
}

func getTermSize() (int, int) {
	ws := struct{ Row, Col, Xpixel, Ypixel uint16 }{}
	syscall.Syscall(syscall.SYS_IOCTL,
		uintptr(syscall.Stdin), uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(&ws)))
	if ws.Col == 0 {
		return 80, 24
	}
	return int(ws.Col), int(ws.Row)
}

func cleanName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == ' ' || r == '\'' || r == '.' {
			b.WriteRune(r)
		}
	}
	return strings.ToLower(strings.TrimSpace(b.String()))
}

func lookupTypes(name string) []string {
	target := cleanName(name)
	if target == "" || target == "unknown" {
		return nil
	}

	if t, ok := PokemonTypes[target]; ok {
		return t
	}
	if t, ok := PokemonTypes[strings.ReplaceAll(target, " ", "-")]; ok {
		return t
	}
	if t, ok := PokemonTypes[strings.ReplaceAll(target, "-", " ")]; ok {
		return t
	}
	noDots := strings.ReplaceAll(target, ".", "")
	if t, ok := PokemonTypes[noDots]; ok {
		return t
	}

	return nil
}

func getInterpolatedRGB(colors []string, offset float64) string {
	if len(colors) == 0 {
		return "255;255;255"
	}
	if len(colors) == 1 {
		return colors[0]
	}
	// offset is 0.0 to 1.0
	n := float64(len(colors))
	idx1 := int(offset*n) % len(colors)
	idx2 := (idx1 + 1) % len(colors)
	frac := (offset * n) - float64(int(offset*n))

	parse := func(s string) (int, int, int) {
		parts := strings.Split(s, ";")
		if len(parts) < 3 {
			return 255, 255, 255
		}
		r, _ := strconv.Atoi(parts[0])
		g, _ := strconv.Atoi(parts[1])
		b, _ := strconv.Atoi(parts[2])
		return r, g, b
	}

	r1, g1, b1 := parse(colors[idx1])
	r2, g2, b2 := parse(colors[idx2])

	r := int(float64(r1) + float64(r2-r1)*frac)
	g := int(float64(g1) + float64(g2-g1)*frac)
	b := int(float64(b1) + float64(b2-b1)*frac)

	return fmt.Sprintf("%d;%d;%d", r, g, b)
}

func formatTypeBadges(types []string, reset string) string {
	var badges []string
	for _, t := range types {
		rgb, ok := typeColors[t]
		if !ok {
			rgb = "180;180;180"
		}
		label := strings.ToUpper(t[:1]) + t[1:]
		badge := fmt.Sprintf("\x1b[1;38;2;255;255;255m\x1b[48;2;%sm %s %s", rgb, label, reset)
		badges = append(badges, badge)
	}
	return strings.Join(badges, " ")
}

// ──────────────── Main Logic ────────────────

func main() {
	rand.Seed(time.Now().UnixNano())
	cfg := loadConfig()

	// 1. Fetch Sprite & Name
	isShiny := rand.Intn(cfg.ShinyChance) == 0
	stats := loadStats()
	if isShiny {
		stats.ShinyCount++
		saveStats(stats)
	}

	args := []string{"random"}
	if isShiny {
		args = append(args, "--shiny")
	}

	// Try to resolve pokeget path
	pokegetPath := "pokeget"
	if _, err := exec.LookPath(pokegetPath); err != nil {
		// Try common locations
		home, _ := os.UserHomeDir()
		candidates := []string{
			filepath.Join(home, ".cargo", "bin", "pokeget"),
			filepath.Join(home, ".local", "bin", "pokeget"),
			"/usr/local/bin/pokeget",
			"/usr/bin/pokeget",
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				pokegetPath = c
				break
			}
		}
	}

	cmd := exec.Command(pokegetPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running pokeget: %v\n", err)
	}
	pokeOut := string(out)

	rawLines := strings.Split(strings.ReplaceAll(pokeOut, "\r", ""), "\n")
	pokemonName := "Unknown"
	nameIdx := -1

	for i, line := range rawLines {
		clean := strings.TrimSpace(stripAnsi(line))
		if len(clean) > 1 && len(clean) < 50 && !strings.ContainsAny(clean, "▄▀█▒░▐▌▀") {
			hasLetter := false
			for _, r := range clean {
				if unicode.IsLetter(r) {
					hasLetter = true
					break
				}
			}
			if hasLetter {
				pokemonName = clean
				nameIdx = i
				break
			}
		}
	}

	var pokeLines []string
	for i, l := range rawLines {
		if i != nameIdx && strings.TrimSpace(l) != "" {
			pokeLines = append(pokeLines, l)
		}
	}

	// 2. Data Retrieval
	types := lookupTypes(pokemonName)
	reset := "\x1b[0m"

	// 3. Fastfetch Info
	var ffRows []string
	shineConfig := os.ExpandEnv("$HOME/.config/shinefetch/fastfetch.jsonc")
	fastfetchConfig := os.ExpandEnv("$HOME/.config/fastfetch/config.jsonc")

	configsToTry := []string{shineConfig, fastfetchConfig}

	for _, cfg := range configsToTry {
		if _, err := os.Stat(cfg); err == nil {
			cmdFF := exec.Command("fastfetch", "-c", cfg, "--logo", "none")
			if out, err := cmdFF.Output(); err == nil {
				ffInfo := string(out)
				if strings.TrimSpace(ffInfo) != "" {
					ffRows = strings.Split(strings.TrimRight(ffInfo, "\n"), "\n")
					break
				}
			}
		}
	}

	// Fallback: No logo pipe mode
	if len(ffRows) == 0 {
		cmdFF := exec.Command("fastfetch", "--logo", "none", "--pipe")
		if out, err := cmdFF.Output(); err == nil {
			ffRows = strings.Split(strings.TrimRight(string(out), "\n"), "\n")
		}
	}

	// 4. Color Extraction
	reColor := regexp.MustCompile(`48;2;(\d+;\d+;\d+)m`)
	matches := reColor.FindAllStringSubmatch(pokeOut, -1)
	counts := make(map[string]int)
	for _, m := range matches {
		counts[m[1]]++
	}
	type CC struct {
		C string
		N int
	}
	var valid, all []CC
	for c, n := range counts {
		all = append(all, CC{c, n})
		rgb := strings.Split(c, ";")
		if len(rgb) == 3 {
			r, _ := strconv.Atoi(rgb[0])
			g, _ := strconv.Atoi(rgb[1])
			b, _ := strconv.Atoi(rgb[2])
			if (r < 30 && g < 30 && b < 30) || (r > 240 && g > 240 && b > 240) {
				continue
			}
			maxC, minC := r, r
			if g > maxC {
				maxC = g
			}
			if b > maxC {
				maxC = b
			}
			if g < minC {
				minC = g
			}
			if b < minC {
				minC = b
			}
			if maxC-minC < 15 {
				continue
			}
			valid = append(valid, CC{c, n})
		}
	}
	sort.Slice(valid, func(i, j int) bool { return valid[i].N > valid[j].N })
	sort.Slice(all, func(i, j int) bool { return all[i].N > all[j].N })

	dom, sec, ter := "32;252;0", "0;255;255", "255;0;255"
	if len(valid) > 0 {
		dom = valid[0].C
		if len(valid) > 1 {
			sec = valid[1].C
		} else if len(all) > 1 {
			sec = all[1].C
		}
		if len(valid) > 2 {
			ter = valid[2].C
		} else if len(all) > 2 {
			ter = all[2].C
		}
	}

	colorDots := ""
	dotSource := append([]CC{}, valid...)
	seen := make(map[string]bool)
	for _, v := range valid {
		seen[v.C] = true
	}
	for _, a := range all {
		if !seen[a.C] && len(dotSource) < 8 {
			dotSource = append(dotSource, a)
			seen[a.C] = true
		}
	}
	for i := 0; i < len(dotSource) && i < 8; i++ {
		colorDots += "\x1b[38;2;" + dotSource[i].C + "m● " + reset
	}

	// 5. Assemble Rows
	speciesVal := pokemonName
	if isShiny {
		speciesVal = "SHINY " + pokemonName + "!!"
	}

	trainer := cfg.TrainerName
	if trainer == "" {
		trainer = os.Getenv("USER")
	}

	var rows []Row
	rows = append(rows, Row{K: "󰦔 Trainer", V: trainer + "@cachyos"})
	rows = append(rows, Row{K: "󰄭 Species", V: speciesVal})

	if len(types) > 0 {
		rows = append(rows, Row{K: "󰓎 Type", V: formatTypeBadges(types, reset), IsRaw: true})
	}

	if stats.ShinyCount > 0 {
		rows = append(rows, Row{K: "󰄳 Caught", V: fmt.Sprintf("%d Shiny Pokemon", stats.ShinyCount)})
	}

	hasFFInfo := false
	var ffInfoRows []Row
	for _, line := range ffRows {
		clean := stripAnsi(line)
		if strings.ContainsAny(clean, "╭╮╰╯├┤─") || strings.TrimSpace(clean) == "" {
			continue
		}

		var k, v string
		if strings.Contains(clean, "│") {
			parts := strings.SplitN(clean, "│", 3)
			if len(parts) >= 2 {
				content := strings.TrimSpace(parts[1])
				if strings.Contains(content, "Trainer") || strings.Contains(content, "Colors") || strings.Contains(content, "Species") {
					continue
				}
				if strings.Contains(content, "➜") {
					sepIdx := strings.Index(content, "➜")
					k = strings.TrimSpace(content[:sepIdx])
					v = strings.TrimSpace(content[sepIdx+len("➜"):])
				}
			}
		} else if strings.Contains(clean, "➜") {
			// Basic key-value format
			sepIdx := strings.Index(clean, "➜")
			k = strings.TrimSpace(clean[:sepIdx])
			v = strings.TrimSpace(clean[sepIdx+len("➜"):])
		}

		if k != "" {
			// Strip icons if any for comparison
			kClean := ""
			for _, r := range k {
				if r < 128 { // Keep basic ASCII
					kClean += string(r)
				}
			}
			kClean = strings.TrimSpace(kClean)
			if kClean == "Trainer" || kClean == "Colors" || kClean == "Species" {
				continue
			}

			ffInfoRows = append(ffInfoRows, Row{K: k, V: v})
			hasFFInfo = true
		}
	}

	if hasFFInfo {
		rows = append(rows, Row{IsSep: true})
		rows = append(rows, ffInfoRows...)
	}

	rows = append(rows, Row{IsSep: true})
	rows = append(rows, Row{K: " Colors", V: colorDots, IsRaw: true})

	// 6. Build Box
	maxK, maxV := 0, 0
	for _, r := range rows {
		if r.IsSep {
			continue
		}
		if l := getVisibleLen(r.K); l > maxK {
			maxK = l
		}
		if l := getVisibleLen(r.V); l > maxV {
			maxV = l
		}
	}

	innerW := maxK + 3 + maxV + 2
	styleKey := cfg.BoxStyle
	if isShiny {
		styleKey = cfg.ShinyBoxStyle
	}
	style := boxStyles[styleKey]
	domC, secC, terC := "\x1b[1;38;2;"+dom+"m", "\x1b[1;38;2;"+sec+"m", "\x1b[1;38;2;"+ter+"m"

	buildBox := func(animOffset float64, borderColors []string) []string {
		totalW := innerW + 2
		totalH := len(rows) + 2

		getB := func(char string, row, col int) string {
			if !isShiny {
				return domC + char + reset
			}

			// Spatial delay: new colors emerge from corners
			// Calculate distance to nearest corner
			dx := math.Min(float64(col), float64(totalW-1-col))
			dy := math.Min(float64(row), float64(totalH-1-row))
			// Corner-centric delay (diagonal distance inward)
			dist := math.Sqrt(dx*dx + dy*dy)

			// Normalize spatial offset (a larger divisor like 300.0 makes color blocks much wider)
			spatialDelay := dist / 300.0

			// Wrap it back into the animation offset to create the spread effect
			o := animOffset - spatialDelay
			for o < 0.0 {
				o += 1.0
			}

			color := getInterpolatedRGB(borderColors, o)

			// Subtler breathing pulse synchronized with the spread
			pulseBase := animOffset * 2.0 * math.Pi * float64(len(borderColors))
			pulse := 0.9 + 0.2*math.Sin(pulseBase-spatialDelay*6.0)

			applyPulse := func(rgb string, p float64) string {
				parts := strings.Split(rgb, ";")
				if len(parts) < 3 {
					return rgb
				}
				r, _ := strconv.Atoi(parts[0])
				g, _ := strconv.Atoi(parts[1])
				b, _ := strconv.Atoi(parts[2])
				r = int(math.Max(0, math.Min(255, float64(r)*p)))
				g = int(math.Max(0, math.Min(255, float64(g)*p)))
				b = int(math.Max(0, math.Min(255, float64(b)*p)))
				return fmt.Sprintf("%d;%d;%d", r, g, b)
			}

			finalColor := applyPulse(color, pulse)
			return "\x1b[1;38;2;" + finalColor + "m" + char + reset
		}

		title := " POKéDEX "
		padT := (innerW - getVisibleLen(title)) / 2

		var bh strings.Builder
		bh.WriteString(getB(style.TL, 0, 0))
		for i := 0; i < padT; i++ {
			bh.WriteString(getB(style.H, 0, 1+i))
		}
		bh.WriteString(title)
		for i := 0; i < max(0, innerW-padT-getVisibleLen(title)); i++ {
			bh.WriteString(getB(style.H, 0, 1+padT+getVisibleLen(title)+i))
		}
		bh.WriteString(getB(style.TR, 0, innerW+1))
		boxHeader := bh.String()

		var bLines []string
		bLines = append(bLines, boxHeader)
		for rIdx, r := range rows {
			rowIdx := rIdx + 1
			if r.IsSep {
				var sb strings.Builder
				sb.WriteString(getB(style.LT, rowIdx, 0))
				for i := 0; i < innerW; i++ {
					sb.WriteString(getB(style.H, rowIdx, 1+i))
				}
				sb.WriteString(getB(style.RT, rowIdx, innerW+1))
				bLines = append(bLines, sb.String())
				continue
			}

			leftV := getB(style.V, rowIdx, 0)
			rightV := getB(style.V, rowIdx, innerW+1)

			line := leftV + " " + reset + terC + r.K + reset + strings.Repeat(" ", maxK-getVisibleLen(r.K)) + " " + domC + "➜" + reset + " "
			if r.IsRaw {
				line += r.V
				curLen := getVisibleLen(line)
				line += strings.Repeat(" ", max(0, innerW-curLen+1)) + rightV
			} else {
				line += secC + r.V + reset + strings.Repeat(" ", maxV-getVisibleLen(r.V)) + " " + rightV
			}
			bLines = append(bLines, line)
		}
		lastRowIdx := len(rows) + 1
		var bf strings.Builder
		bf.WriteString(getB(style.BL, lastRowIdx, 0))
		for i := 0; i < innerW; i++ {
			bf.WriteString(getB(style.H, lastRowIdx, 1+i))
		}
		bf.WriteString(getB(style.BR, lastRowIdx, innerW+1))
		bLines = append(bLines, bf.String())
		return bLines
	}

	// 7. Interactive Render Loop
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		// Fallback to stdout for basic display
		shinyColors := []string{}
		for _, cc := range dotSource {
			shinyColors = append(shinyColors, cc.C)
		}
		boxLines := buildBox(0, shinyColors)
		maxH := max(len(pokeLines), len(boxLines))
		pTop, bTop := (maxH-len(pokeLines))/2, (maxH-len(boxLines))/2
		for i := 0; i < maxH; i++ {
			pStr, bStr := "", ""
			if idx := i - pTop; idx >= 0 && idx < len(pokeLines) {
				pStr = pokeLines[idx]
			}
			if idx := i - bTop; idx >= 0 && idx < len(boxLines) {
				bStr = boxLines[idx]
			}
			// Manual padding because %-40s breaks with ANSI codes
			pVisible := getVisibleLen(pStr)
			pPad := ""
			if pVisible < 40 {
				pPad = strings.Repeat(" ", 40-pVisible)
			}
			fmt.Printf("%s%s    %s\n", pStr, pPad, bStr)
		}
		return
	}
	defer tty.Close()

	// Hide cursor and ensure we have enough height
	tty.WriteString("\x1b[?25l")

	pokeW := 0
	for _, l := range pokeLines {
		if v := getVisibleLen(l); v > pokeW {
			pokeW = v
		}
	}

	// Pre-calculation for stability
	_, termH := getTermSize()

	// Pre-pad with maxH newlines and move back up to "reserve" space.
	// This prevents "climbing" duplicates when at the bottom of the terminal.
	boxLinesTmp := buildBox(0, []string{"0;0;0"}) 
	maxHTmp := max(len(pokeLines), len(boxLinesTmp))

	vPadTmp := 0
	if cfg.Align == "center" {
		vPadTmp = (termH - maxHTmp) / 8
		if vPadTmp < 1 {
			vPadTmp = 1
		}
	} else {
		vPadTmp = 2
	}

	for i := 0; i < vPadTmp+maxHTmp; i++ {
		tty.WriteString("\n")
	}
	tty.WriteString(fmt.Sprintf("\x1b[%dA", vPadTmp+maxHTmp)) 
	tty.WriteString("\x1b[s")                                 

	defer tty.WriteString("\x1b[?25h")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGWINCH)

	var shinyColors []string
	for _, cc := range dotSource {
		shinyColors = append(shinyColors, cc.C)
	}
	// If it's a very monochromatic sprite, add some variety or just fallback
	if len(shinyColors) < 2 {
		shinyColors = append(shinyColors, "255;255;255")
	}

	render := func(animOffset float64) {
		termW, termH := getTermSize()
		gap := cfg.Gap
		lPad := 0
		if cfg.Align == "center" {
			totalW := pokeW + gap + innerW + 2
			if totalW > termW {
				gap = max(2, gap-(totalW-termW))
				totalW = pokeW + gap + innerW + 2
			}
			lPad = max(0, (termW-totalW)/2)
		} else {
			totalW := gap + pokeW + gap + innerW + 2
			if totalW > termW {
				gap = max(2, gap-(totalW-termW)/2)
			}
			lPad = gap
		}
		lPadS := strings.Repeat(" ", lPad)

		boxLines := buildBox(animOffset, shinyColors)
		maxH := max(len(pokeLines), len(boxLines))

		vPad := 0
		if cfg.Align == "center" {
			vPad = (termH - maxH) / 8
			if vPad < 1 {
				vPad = 1
			}
		} else {
			vPad = 2
		}

		// Return to saved position
		if !cfg.PrintAndExit {
			tty.WriteString("\x1b[u")
			tty.WriteString(strings.Repeat("\n", vPad))
		}

		pTop, bTop := (maxH-len(pokeLines))/2, (maxH-len(boxLines))/2
		for i := 0; i < maxH; i++ {
			pStr := strings.Repeat(" ", pokeW)
			if idx := i - pTop; idx >= 0 && idx < len(pokeLines) {
				pStr = strings.Repeat(" ", (pokeW-getVisibleLen(pokeLines[idx]))/2) + pokeLines[idx]
				pStr += strings.Repeat(" ", pokeW-getVisibleLen(pStr))
			}
			bStr := ""
			if idx := i - bTop; idx >= 0 && idx < len(boxLines) {
				bStr = boxLines[idx]
			}

			lineOut := lPadS + pStr + strings.Repeat(" ", gap) + bStr
			if !cfg.PrintAndExit {
				// Clear line, print, and move to absolute next line WITHOUT scrolling
				// \r = home, \x1b[2K = clear line, \x1b[1B = move down 1
				tty.WriteString("\r\x1b[2K" + lineOut)
				if i < maxH-1 {
					tty.WriteString("\n")
				}
			} else {
				fmt.Println(lineOut)
			}
		}
		// Clear below just in case
		if !cfg.PrintAndExit {
			tty.WriteString("\x1b[J")
		}
	}

	if cfg.PrintAndExit {
		render(0.0)
		return
	}

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	animOffset := 0.0
	render(animOffset)

	keyChan := make(chan struct{}, 1)
	go func() {
		fd := int(tty.Fd())
		var old syscall.Termios
		// TCGETS = 0x5401, TCSETS = 0x5402
		syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), 0x5401, uintptr(unsafe.Pointer(&old)))
		newState := old
		newState.Lflag &^= syscall.ECHO | syscall.ICANON
		syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), 0x5402, uintptr(unsafe.Pointer(&newState)))

		var b [1]byte
		tty.Read(b[:])

		// Restore terminal first
		syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), 0x5402, uintptr(unsafe.Pointer(&old)))

		// TIOCSTI (0x5412) - Re-insert the character into the TTY input buffer
		// so it shows up on the new line in the shell prompt.
		if b[0] != 0 && b[0] != 27 && b[0] != 13 { // Don't re-insert null, Esc, or Enter
			syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), 0x5412, uintptr(unsafe.Pointer(&b[0])))
		}

		keyChan <- struct{}{}
	}()

	for {
		select {
		case <-sigChan:
			render(animOffset)
		case <-ticker.C:
			if isShiny || cfg.Animation {
				animOffset += 0.0017
				if animOffset > 1.0 {
					animOffset -= 1.0
				}
				render(animOffset)
			}
		case <-keyChan:
			tty.WriteString("\x1b[?25h\n")
			return
		}
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
