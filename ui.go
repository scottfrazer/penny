package main

import (
	"fmt"
	"github.com/gdamore/tcell"
	"time"
)

type BorderRunes struct {
	TopLeft     rune
	TopRight    rune
	BottomLeft  rune
	BottomRight rune
	TopBottom   rune
	Sides       rune
}

func BorderNormal(s tcell.Screen, title string, x, y, w, h int) {
	Border(s, title, x, y, w, h, BorderRunes{'┌', '┐', '└', '┘', '━', '│'})
	//Border(s, title, x, y, w, h, BorderRunes{'+', '+', '+', '+', '-', '|'})
}

func BorderNone(s tcell.Screen, x, y, w, h int) {
	Border(s, "", x, y, w, h, BorderRunes{' ', ' ', ' ', ' ', ' ', ' '})
}

func Border(s tcell.Screen, title string, x, y, w, h int, runes BorderRunes) {
	st := tcell.StyleDefault
	s.SetContent(x, y, runes.TopLeft, nil, st)
	s.SetContent(x+w-1, y, runes.TopRight, nil, st)
	s.SetContent(x, y+h-1, runes.BottomLeft, nil, st)
	s.SetContent(x+w-1, y+h-1, runes.BottomRight, nil, st)
	for i := x + 1; i < x+w-1; i++ {
		s.SetContent(i, y, runes.TopBottom, nil, st)
		s.SetContent(i, y+h-1, runes.TopBottom, nil, st)
	}
	draw_string(s, title, x+2, y, st)
	for i := y + 1; i < y+h-1; i++ {
		s.SetContent(x, i, runes.Sides, nil, st)
		s.SetContent(x+w-1, i, runes.Sides, nil, st)
	}
}

func draw_string(screen tcell.Screen, s string, x, y int, style tcell.Style) {
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
}

type UITable struct {
	parent   UIElement
	title    string
	window   Window
	rows     []string
	top      int
	selected int
	x        int
	y        int
	w        int
	h        int
}

func (ui *UITable) Render(screen tcell.Screen) {
	BorderNormal(screen, ui.title, ui.x, ui.y, ui.w, ui.h)
	y := ui.y + 1
	for i := ui.top; i < len(ui.rows) && i < ui.h-2+ui.top; i++ {
		row := ui.rows[i]
		st := tcell.StyleDefault
		if i == ui.selected {
			st = st.Background(tcell.NewRGBColor(-1, 0, 0))
		}
		if len(row) > ui.w {
			row = row[:ui.w]
		}
		draw_string(screen, row, ui.x+1, y, st)
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

func (ui *UITable) Up(i int) {
	if ui.selected-i < ui.top && ui.top != 0 {
		ui.top = Max(ui.top-i, 0)
	}
	if ui.selected > 0 {
		ui.selected = Max(ui.selected-i, 0)
	}
}

func (ui *UITable) Down(i int) {
	if ui.selected < len(ui.rows)-1 {
		if ui.selected-ui.top+i >= ui.h-3 {
			ui.top = Min(ui.top+i, len(ui.rows)-1)
		}
		ui.selected = Min(ui.selected+i, len(ui.rows)-1)
	}
}

func (ui *UITable) Bottom() {
	ui.selected = len(ui.rows) - 1
	ui.top = 0
	if len(ui.rows) > ui.h-2 {
		ui.top = len(ui.rows) - ui.h + 2
	}
}

func (ui *UITable) Top() {
	ui.selected = 0
	ui.top = 0
}

func (ui *UITable) Handle(e tcell.Event) bool {
	switch e := e.(type) {
	case *tcell.EventKey:
		switch e.Key() {
		case tcell.KeyUp:
			ui.Up(1)
		case tcell.KeyDown:
			ui.Down(1)
		case tcell.KeyCtrlU:
			ui.Up(20)
		case tcell.KeyCtrlD:
			ui.Down(20)
		case tcell.KeyRune:
			switch e.Rune() {
			case '1':
				ui.Top()
			case 'j':
				ui.Down(1)
			case 'k':
				ui.Up(1)
			case 'G':
				ui.Bottom()
			}
		}
	}
	return true
}

func (ui *UITable) Parent() UIElement {
	return ui.parent
}

type UIPopupInput struct {
	parent  UIElement
	window  Window
	buffer  string
	result  chan string
	visible bool
	w       int
	h       int
}

func (ui *UIPopupInput) Parent() UIElement {
	return ui.parent
}

func (ui *UIPopupInput) Render(screen tcell.Screen) {
	if ui.visible {
		w, h := screen.Size()
		px := (w - ui.w) / 2
		py := (h - ui.h) / 2
		BorderNormal(screen, "Input", px, py, ui.w, ui.h)
		draw_string(screen, ui.buffer+"_", px+1, py+1, tcell.StyleDefault)
	}
}

func (ui *UIPopupInput) Handle(ev tcell.Event) bool {
	switch ev := ev.(type) {
	case *tcell.EventKey:
		switch ev.Key() {
		case tcell.KeyEnter:
			ui.result <- ui.buffer
			fallthrough
		case tcell.KeyEscape:
			ui.visible = false
			ui.buffer = ""
			ui.window.Unfocus(ui)
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

func (ui *UIPopupInput) GetText() string {
	return ""
}

type Window interface {
	Unfocus(e UIElement)
}

type PennyScreen struct {
	screen     tcell.Screen
	results    chan string
	popup      *UIPopupInput
	table      *UITable
	table2     *UITable
	categories *UITable
	focus      []UIElement
	quit       chan struct{}
	key        *tcell.EventKey
}

func (ui *PennyScreen) Unfocus(e UIElement) {
	top := len(ui.focus) - 1
	if ui.focus[top] == e {
		ui.focus = ui.focus[:top]
	}
}

func (ui *PennyScreen) Parent() UIElement {
	return nil
}

func NewPennyScreen(screen tcell.Screen, rows []string) *PennyScreen {
	results := make(chan string)
	ps := &PennyScreen{screen, results, nil, nil, nil, nil, nil, make(chan struct{}), nil}
	popup := &UIPopupInput{ps, ps, "", nil, false, 40, 3}
	table := &UITable{ps, "Transactions", ps, rows, 0, 0, 0, 0, 0, 0}
	table2 := &UITable{ps, "Debug", ps, []string{}, 0, 0, 0, 0, 0, 0}
	categories := &UITable{ps, "Categories", ps, []string{}, 0, 0, 0, 0, 0, 0}
	ps.popup = popup
	ps.table = table
	ps.table2 = table2
	ps.categories = categories
	ps.focus = []UIElement{table}
	return ps
}

func (ps *PennyScreen) Display() {
	go func() {
		for {
			e := ps.screen.PollEvent()
			for element := ps.focus[len(ps.focus)-1]; element != nil; element = element.Parent() {
				if element.Handle(e) == false {
					break
				}
			}
		}
	}()

	go func() {
		for {
			select {
			case s := <-ps.results:
				ps.table.rows = append(ps.table.rows, s)
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
	w, h := screen.Size()
	ps.table.w = w / 2
	ps.table.h = h
	ps.table.Render(screen)

	ps.table2.x = w / 2
	ps.table2.y = 0
	ps.table2.w = w / 2
	ps.table2.h = 10
	ps.table2.rows = []string{
		fmt.Sprintf("selected=%d", ps.table.selected),
		fmt.Sprintf("rows=%d", len(ps.table.rows)),
		fmt.Sprintf("top=%d", ps.table.top),
		fmt.Sprintf("h=%d", ps.table.h),
		fmt.Sprintf("window w=%d", w),
		fmt.Sprintf("window h=%d", h),
	}
	if ps.key != nil {
		ps.table2.rows = append(ps.table2.rows, fmt.Sprintf("key=%s, mod=%d", ps.key.Name(), ps.key.Modifiers()))
	}
	ps.table2.Render(screen)

	ps.categories.x = w / 2
	ps.categories.y = 10
	ps.categories.w = w / 2
	ps.categories.h = h - 10
	ps.categories.Render(screen)

	ps.popup.Render(screen)
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
		case tcell.KeyCtrlI:
			ps.popup.visible = !ps.popup.visible
			if ps.popup.visible {
				ps.popup.result = ps.results
				ps.focus = append(ps.focus, ps.popup)
			}
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
