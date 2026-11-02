//go:build !windows && gui

package main

import (
	"strconv"
	"strings"
	"testing"

	"fyne.io/fyne/v2/widget"
)

func BenchmarkStudioCatalog(b *testing.B) {
	for _, action := range []string{"Search", "SelectAll", "EditSetting"} {
		b.Run(action, func(b *testing.B) {
			g := newStudioTest(b)
			var search *widget.Entry
			for _, object := range studioTestObjects(g.settings.Items[0].Content) {
				if field, ok := object.(*widget.Entry); ok && strings.HasPrefix(field.PlaceHolder, "Поиск:") {
					search = field
				}
			}
			all := findStudioTestButton(g.settings.Items[0].Content, "Все блоки")
			clear := findStudioTestButton(g.settings.Items[0].Content, "Снять выбор")
			queries := []string{"", "б", "бе", "бет", "бетон", "poplar", "deepslate", "гриб"}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				switch action {
				case "Search":
					search.SetText(queries[i%len(queries)])
				case "SelectAll":
					if i%2 == 0 {
						all.OnTapped()
					} else {
						clear.OnTapped()
					}
				case "EditSetting":
					g.path.SetText("image_" + strconv.Itoa(i) + ".png")
				}
			}
		})
	}
}
