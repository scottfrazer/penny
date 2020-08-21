package main

import (
	"fmt"
	"github.com/gdamore/tcell"
	"regexp"
	"strings"
	"time"
)

type UIBorder struct {
	TopLeft     rune
	TopRight    rune
	BottomLeft  rune
	BottomRight rune
	TopBottom   rune
	Sides       rune
	Style       tcell.Style
	FocusStyle  tcell.Style
}

func DefaultUIBorder() UIBorder {
	return UIBorder{
		'┌',
		'┐',
		'└',
		'┘',
		'━',
		'│',
		tcell.StyleDefault,
		tcell.StyleDefault.Foreground(tcell.ColorGreen),
	}
}

func BorderNormal(s tcell.Screen, title string, x, y, w, h int, focus bool) {
	Border(s, title, x, y, w, h, focus, DefaultUIBorder())
}

func Border(s tcell.Screen, title string, x, y, w, h int, focus bool, runes UIBorder) {
	st := runes.Style
	if focus {
		st = runes.FocusStyle
	}

	s.SetContent(x, y, runes.TopLeft, nil, st)
	s.SetContent(x+w-1, y, runes.TopRight, nil, st)
	s.SetContent(x, y+h-1, runes.BottomLeft, nil, st)
	s.SetContent(x+w-1, y+h-1, runes.BottomRight, nil, st)
	for i := x + 1; i < x+w-1; i++ {
		s.SetContent(i, y, runes.TopBottom, nil, st)
		s.SetContent(i, y+h-1, runes.TopBottom, nil, st)
	}
	DrawString(s, title, x+2, y, runes.Style)
	for i := y + 1; i < y+h-1; i++ {
		s.SetContent(x, i, runes.Sides, nil, st)
		s.SetContent(x+w-1, i, runes.Sides, nil, st)
	}
}

func Box(s tcell.Screen, x, y, w, h int, style tcell.Style) {
	for i := x; i < x+w; i++ {
		for j := y; j < y+h; j++ {
			s.SetContent(i, j, ' ', nil, style)
		}
	}
}

func DrawString(screen tcell.Screen, s string, x, y int, style tcell.Style) {
	i := x
	for _, r := range s {
		screen.SetContent(i, y, r, nil, style)
		i++
	}
}

type UIElement interface {
	Handle(event tcell.Event) bool
	Parent() UIElement
	Render(screen tcell.Screen)
	X() int
	Y() int
	Width() int
	Height() int
}

type UITable struct {
	parent     UIElement
	root       Window
	title      string
	rows       func() []string
	enter      func(*UITable)
	top        int
	selected   int
	x, y, w, h int
}

func (ui *UITable) X() int {
	return ui.x
}

func (ui *UITable) Y() int {
	return ui.y
}

func (ui *UITable) Width() int {
	return ui.w
}

func (ui *UITable) Height() int {
	return ui.h
}

func (ui *UITable) Render(screen tcell.Screen) {
	BorderNormal(screen, ui.title, ui.x, ui.y, ui.w, ui.h, ui.root.IsFocus(ui))
	y := ui.y + 1
	rows := ui.rows()
	for i := ui.top; i < len(rows) && i < ui.h-2+ui.top; i++ {
		row := rows[i]
		st := tcell.StyleDefault
		if i == ui.selected && ui.root.IsFocus(ui) {
			st = st.Background(tcell.NewRGBColor(-1, 0, 0))
		}
		if len(row) > ui.w {
			row = row[:ui.w]
		}
		DrawString(screen, row, ui.x+1, y, st)
		y++
	}
}

func Max(i, j int) int {
	if i > j {
		return i
	}
	return j
}

func Min(i, j int) int {
	if i < j {
		return i
	}
	return j
}

func (ui *UITable) Up(i int) bool {
	original := ui.selected
	if ui.selected-i < ui.top && ui.top != 0 {
		ui.top = Max(ui.top-i, 0)
	}
	if ui.selected > 0 {
		ui.selected = Max(ui.selected-i, 0)
	}
	return original == ui.selected
}

func (ui *UITable) Down(i int) bool {
	original := ui.selected
	rows := ui.rows()
	if ui.selected < len(rows)-1 {
		if ui.selected-ui.top+i >= ui.h-3 {
			ui.top = Min(ui.top+i, len(rows)-1)
		}
		ui.selected = Min(ui.selected+i, len(rows)-1)
	}
	return original == ui.selected
}

func (ui *UITable) Bottom() bool {
	original := ui.selected
	rows := ui.rows()
	ui.selected = len(rows) - 1
	ui.top = 0
	if len(rows) > ui.h-2 {
		ui.top = len(rows) - ui.h + 2
	}
	return original == ui.selected
}

func (ui *UITable) Top() bool {
	original := ui.selected
	ui.selected = 0
	ui.top = 0
	return original == ui.selected
}

func (ui *UITable) Handle(e tcell.Event) bool {
	switch e := e.(type) {
	case *tcell.EventKey:
		switch e.Key() {
		case tcell.KeyEnter:
			if ui.enter != nil {
				ui.enter(ui)
			}
		case tcell.KeyUp:
			return ui.Up(1)
		case tcell.KeyDown:
			return ui.Down(1)
		case tcell.KeyCtrlU:
			return ui.Up(20)
		case tcell.KeyCtrlD:
			return ui.Down(20)
		case tcell.KeyRune:
			switch e.Rune() {
			case '1':
				return ui.Top()
			case 'j':
				return ui.Down(1)
			case 'k':
				return ui.Up(1)
			case 'G':
				return ui.Bottom()
			}
		}
	}
	return true
}

func (ui *UITable) Parent() UIElement {
	return ui.parent
}

type UIPopupInput struct {
	parent   UIElement
	window   Window
	buffer   string
	callback func(string)
	close    func()
	visible  bool
	w        int
	h        int
}

func (ui *UIPopupInput) X() int {
	return (ui.parent.Width() - ui.w) / 2
}

func (ui *UIPopupInput) Y() int {
	return (ui.parent.Height() - ui.h) / 2
}

func (ui *UIPopupInput) Width() int {
	return ui.w
}

func (ui *UIPopupInput) Height() int {
	return ui.h
}

func (ui *UIPopupInput) Parent() UIElement {
	return ui.parent
}

func (ui *UIPopupInput) Render(screen tcell.Screen) {
	if ui.visible {
		Box(screen, ui.X()-2, ui.Y()-1, ui.Width()+4, ui.Height()+2, tcell.StyleDefault.Background(tcell.ColorGray))
		border := DefaultUIBorder()
		border.Style = tcell.StyleDefault.Background(tcell.ColorGray)
		border.FocusStyle = tcell.StyleDefault.Background(tcell.ColorGray).Foreground(tcell.ColorGreen)
		Border(screen, "Input", ui.X(), ui.Y(), ui.Width(), ui.Height(), true, border)
		//Box(screen, ui.X()+1, ui.Y()+1, ui.Width()-2, ui.Height()-2, tcell.StyleDefault)
		DrawString(screen, ui.buffer+"_", ui.X()+1, ui.Y()+2, border.Style)
	}
}

func (ui *UIPopupInput) Handle(ev tcell.Event) bool {
	switch ev := ev.(type) {
	case *tcell.EventKey:
		switch ev.Key() {
		case tcell.KeyEnter:
			ui.callback(ui.buffer)
			fallthrough
		case tcell.KeyEscape:
			ui.visible = false
			ui.buffer = ""
			ui.close()
		case tcell.KeyBackspace2:
			if last := len(ui.buffer) - 1; last >= 0 {
				ui.buffer = ui.buffer[:last]
			}
		case tcell.KeyRune:
			ui.buffer = ui.buffer + string(ev.Rune())
		}
	}
	return false
}

type Window interface {
	IsFocus(e UIElement) bool
	SetFocus(e UIElement)
}

type TxWindow struct {
	parent       UIElement
	root         Window
	transactions *UITable
	categories   *UITable
	criteria     *UITable
	debug        *UITable
	popupInput   *UIPopupInput
	x, y, w, h   int
}

func (ui *TxWindow) Render(screen tcell.Screen) {
	ui.transactions.x = ui.parent.X()
	ui.transactions.y = ui.parent.Y()
	ui.transactions.w = ui.parent.Width() / 2
	ui.transactions.h = ui.parent.Height()
	ui.criteria.x = ui.parent.X() + ui.parent.Width()/2
	ui.criteria.y = ui.parent.Y()
	ui.criteria.w = ui.parent.Width() / 2
	ui.criteria.h = 10
	ui.debug.x = ui.parent.X() + ui.parent.Width()/2
	ui.debug.y = ui.parent.Y()
	ui.debug.w = ui.parent.Width() / 2
	ui.debug.h = 10
	ui.categories.x = ui.parent.X() + ui.parent.Width()/2
	ui.categories.y = ui.parent.Y() + ui.criteria.h
	ui.categories.w = ui.parent.Width() / 2
	ui.categories.h = ui.parent.Height() - ui.criteria.h

	ui.categories.Render(screen)
	ui.transactions.Render(screen)
	ui.criteria.Render(screen)
	ui.popupInput.Render(screen)
}

func (ui *TxWindow) Handle(ev tcell.Event) bool {
	switch ev := ev.(type) {
	case *tcell.EventKey:
		switch ev.Key() {
		case tcell.KeyRight:
			if ui.root.IsFocus(ui.transactions) {
				ui.root.SetFocus(ui.criteria)
			}
		case tcell.KeyLeft:
			if ui.root.IsFocus(ui.criteria) || ui.root.IsFocus(ui.categories) {
				ui.root.SetFocus(ui.transactions)
			}
		case tcell.KeyDown:
			if ui.root.IsFocus(ui.criteria) {
				ui.root.SetFocus(ui.categories)
			}
		case tcell.KeyUp:
			if ui.root.IsFocus(ui.categories) {
				ui.root.SetFocus(ui.criteria)
			}
		}
	}
	return true
}

func (ui *TxWindow) Parent() UIElement {
	return ui.parent
}

func (ui *TxWindow) X() int {
	return ui.x
}

func (ui *TxWindow) Y() int {
	return ui.y
}

func (ui *TxWindow) Width() int {
	return ui.w
}

func (ui *TxWindow) Height() int {
	return ui.h
}

type PennyScreen struct {
	screen     tcell.Screen
	db         *PennyDb
	start      time.Time
	end        time.Time
	regex      *regexp.Regexp
	categories []string
	slice      *TxSlice // cached slice from `db` based on the above parameters
	txWindow   *TxWindow
	focus      UIElement
	quit       chan struct{}
	key        *tcell.EventKey
	w          int
	h          int
}

func (ui *PennyScreen) SetFocus(e UIElement) {
	ui.focus = e
}

func (ui *PennyScreen) IsFocus(e UIElement) bool {
	return ui.focus == e
}

func (ui *PennyScreen) X() int {
	return 0
}

func (ui *PennyScreen) Y() int {
	return 0
}

func (ui *PennyScreen) Width() int {
	return ui.w
}

func (ui *PennyScreen) Height() int {
	return ui.h
}

func (ui *PennyScreen) Parent() UIElement {
	return nil
}

func NewPennyScreen(screen tcell.Screen, db *PennyDb, start, end time.Time, regex *regexp.Regexp, categories []string) *PennyScreen {
	ps := &PennyScreen{
		screen:     screen,
		db:         db,
		start:      start,
		end:        end,
		regex:      regex,
		categories: categories,
		quit:       make(chan struct{}),
	}

	ps.txWindow = &TxWindow{
		transactions: &UITable{
			title: "Transactions",
			rows:  ps.TxRows,
		},
		categories: &UITable{
			title: "Categories",
			rows:  ps.CategoryRows,
		},
		criteria: &UITable{
			title: "Criteria",
			rows: func() []string {
				return []string{
					fmt.Sprintf("Date Range: %s - %s", ps.start.Format("01/02/2006"), ps.end.Format("01/02/2006")),
					fmt.Sprintf("Categories: %s", strings.Join(ps.categories, ", ")),
					fmt.Sprintf("Regex: %s", ps.regex.String()),
				}
			},
			enter: func(table *UITable) {
				switch table.selected {
				case 0:
					popup := ps.txWindow.popupInput
					popup.buffer = fmt.Sprintf("%s - %s", ps.start.Format("01/02/2006"), ps.end.Format("01/02/2006"))
					popup.callback = func(s string) {
						start, end, err := ParseDateRange(s)
						if err != nil {
							panic(err)
						}
						ps.start = start
						ps.end = end
						ps.Load()
					}
					popup.close = func() {
						ps.focus = ps.txWindow.transactions
					}
					popup.visible = true
					ps.focus = popup
				case 1:
					popup := ps.txWindow.popupInput
					popup.buffer = strings.Join(ps.categories, ", ")
					popup.callback = func(s string) {
						ps.categories = []string{}
						if len(strings.TrimSpace(s)) > 0 {
							for _, category := range strings.Split(s, ",") {
								ps.categories = append(ps.categories, strings.TrimSpace(category))
							}
						}
						ps.Load()
					}
					popup.close = func() {
						ps.focus = ps.txWindow.transactions
					}
					popup.visible = true
					ps.focus = popup
				case 2:
					popup := ps.txWindow.popupInput
					popup.buffer = ps.regex.String()
					popup.callback = func(s string) {
						ps.regex = regexp.MustCompile(s)
						ps.Load()
					}
					popup.close = func() {
						ps.focus = ps.txWindow.transactions
					}
					popup.visible = true
					ps.focus = popup
				}
			},
		},
		debug: &UITable{
			title: "Debug",
			rows:  func() []string { return []string{} },
		},
		popupInput: &UIPopupInput{
			w: 60,
			h: 5,
		},
	}

	ps.txWindow.parent = ps
	ps.txWindow.root = ps

	ps.txWindow.popupInput.parent = ps.txWindow
	for _, elem := range []*UITable{ps.txWindow.transactions, ps.txWindow.categories, ps.txWindow.criteria, ps.txWindow.debug} {
		elem.parent = ps.txWindow
		elem.root = ps
	}
	ps.focus = ps.txWindow.transactions
	ps.Load()
	return ps
}

func (ps *PennyScreen) Load() {
	ps.slice = ps.db.Slice(ps.start, ps.end, ps.regex, ps.categories)
}

func (ps *PennyScreen) TxRows() []string {
	return ps.slice.TableRows(false)
}

func (ps *PennyScreen) CategoryRows() []string {
	rows := []string{}
	netTransactions := 0
	netAmount := 0.0
	elapsedDays := ps.slice.ElapsedDays()
	for _, summary := range ps.slice.CategorySummaries() {
		netAmount += summary.Total
		netTransactions += summary.TransactionCount
		perDay := summary.Total / elapsedDays
		rows = append(rows, fmt.Sprintf(
			"%15s %3d %10s %10s %10s %s %.2f%%",
			summary.Category,
			summary.TransactionCount,
			money(summary.Total, false),
			money(perDay, false),
			money(perDay*7, false),
			money(perDay*30, false),
			summary.PercentageOfIncome))
	}
	return rows
}

func (ps *PennyScreen) Display() {
	go func() {
		for {
			e := ps.screen.PollEvent()
			if e != nil {
				for element := ps.focus; element != nil; element = element.Parent() {
					if element.Handle(e) == false {
						break
					}
				}
			}
		}
	}()

loop:
	for {
		select {
		case <-ps.quit:
			break loop
		case <-time.After(time.Millisecond * 50):
		}
		ps.Redraw()
	}

	ps.screen.Fini()
}

func (ps *PennyScreen) Render(screen tcell.Screen) {
	DrawString(ps.screen, "this is a title", 0, 0, tcell.StyleDefault)
	w, h := screen.Size()
	ps.w = w
	ps.h = h

	ps.txWindow.x = 1
	ps.txWindow.y = 0
	ps.txWindow.w = ps.w
	ps.txWindow.h = ps.h
	ps.txWindow.Render(screen)

	ps.txWindow.debug.rows = func() []string {
		r := []string{
			fmt.Sprintf("selected=%d", ps.txWindow.transactions.selected),
			fmt.Sprintf("top=%d", ps.txWindow.transactions.top),
			fmt.Sprintf("h=%d", ps.txWindow.h),
			fmt.Sprintf("rows=%d", len(ps.slice.transactions)),
			fmt.Sprintf("window w=%d", w),
			fmt.Sprintf("window h=%d", h),
		}
		if ps.key != nil {
			r = append(r, fmt.Sprintf("key=%s, mod=%d", ps.key.Name(), ps.key.Modifiers()))
		}
		return r
	}
}

func (ps *PennyScreen) Redraw() {
	ps.screen.Clear()
	ps.Render(ps.screen)
	ps.screen.Show()
}

func (ps *PennyScreen) Handle(ev tcell.Event) bool {
	switch ev := ev.(type) {
	case *tcell.EventKey:
		ps.key = ev

		switch ev.Key() {
		case tcell.KeyEscape:
			close(ps.quit)
		case tcell.KeyRune:
			switch ev.Rune() {
			case 'q':
				close(ps.quit)
			}
		}
	case *tcell.EventResize:
		ps.screen.Sync()
	}
	return false
}

func ParseDateRange(s string) (time.Time, time.Time, error) {
	var start, end time.Time
	var err error

	re := regexp.MustCompile(`(\d\d\/\d\d\/\d\d\d\d)\s+\-\s+(\d\d\/\d\d\/\d\d\d\d)`)
	m := re.FindAllStringSubmatch(s, -1)
	if len(m) > 0 {
		start, err = time.Parse("01/02/2006", m[0][1])
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		end, err = time.Parse("01/02/2006", m[0][2])
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		return start, end, nil
	}

	re = regexp.MustCompile(`[Qq](\d)\s*(\d{4})`)
	m = re.FindAllStringSubmatch(s, -1)
	if len(m) > 0 {
		q := m[0][1]
		y := m[0][2]
		var s, e string
		switch q {
		case "1":
			s = "01/01"
			e = "03/31"
		case "2":
			s = "04/01"
			e = "06/30"
		case "3":
			s = "07/01"
			e = "09/30"
		case "4":
			s = "10/01"
			e = "12/31"
		}

		start, err = time.Parse("01/02/2006", fmt.Sprintf("%s/%s", s, y))
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		end, err = time.Parse("01/02/2006", fmt.Sprintf("%s/%s", e, y))
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		return start, end, nil
	}

	return time.Time{}, time.Time{}, fmt.Errorf("Invalid date range: " + s)
}
