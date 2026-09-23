// tools/icon 把 SVG 图标转换为多尺寸 Windows .ico 文件。
//
// 用法：
//
//	go run ./tools/icon -in mcp-icon.svg -out internal/app/icon.ico
//
// 生成的 .ico 通过 github.com/akavel/rsrc 嵌入 exe（见 build.ps1），
// 使资源管理器、任务栏和 WebView2 窗口都显示该图标。
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/tdewolff/canvas"
	"github.com/tdewolff/canvas/renderers"
)

// iconSizes 是 ICO 内嵌的位图尺寸（覆盖 16~256，适配资源管理器/任务栏/Alt+Tab）。
var iconSizes = []int{16, 20, 24, 32, 40, 48, 64, 256}

func main() {
	in := flag.String("in", "", "输入 SVG 文件路径")
	out := flag.String("out", "", "输出 .ico 文件路径")
	flag.Parse()
	if *in == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "用法: go run ./tools/icon -in icon.svg -out icon.ico")
		os.Exit(2)
	}

	raw, err := os.ReadFile(*in)
	if err != nil {
		fatal("读取 SVG 失败: %v", err)
	}
	// canvas 不解析 CSS 的 color 属性，currentColor 需替换为具体颜色
	svg := strings.NewReplacer(
		`fill="currentColor"`, `fill="#000000"`,
		`fill='currentColor'`, `fill='#000000'`,
	).Replace(string(raw))

	c, err := canvas.ParseSVG(strings.NewReader(svg))
	if err != nil {
		fatal("解析 SVG 失败: %v", err)
	}
	if c.W <= 0 || c.H <= 0 {
		fatal("SVG 尺寸异常: %vx%v", c.W, c.H)
	}

	type entry struct {
		size int
		data []byte
	}
	var entries []entry
	for _, s := range iconSizes {
		var buf bytes.Buffer
		// Canvas 单位为毫米；分辨率 = 目标像素数 / 宽度(毫米)
		res := canvas.DPMM(float64(s) / c.W)
		if err := c.Write(&buf, renderers.PNG(res)); err != nil {
			fatal("渲染 %dpx 失败: %v", s, err)
		}
		if buf.Len() == 0 {
			fatal("渲染 %dpx 结果为空", s)
		}
		entries = append(entries, entry{size: s, data: buf.Bytes()})
	}

	// 组装 ICO：ICONDIR + 若干 ICONDIRENTRY + PNG 数据
	var ico bytes.Buffer
	ico.Write([]byte{0, 0}) // reserved
	binary.Write(&ico, binary.LittleEndian, uint16(1))
	binary.Write(&ico, binary.LittleEndian, uint16(len(entries)))
	offset := 6 + 16*len(entries)
	for _, e := range entries {
		w, h := byte(e.size), byte(e.size)
		if e.size >= 256 {
			w, h = 0, 0 // ICO 规范：256 用 0 表示
		}
		ico.WriteByte(w)
		ico.WriteByte(h)
		ico.WriteByte(0) // 调色板颜色数
		ico.WriteByte(0) // reserved
		binary.Write(&ico, binary.LittleEndian, uint16(1))
		binary.Write(&ico, binary.LittleEndian, uint16(32))
		binary.Write(&ico, binary.LittleEndian, uint32(len(e.data)))
		binary.Write(&ico, binary.LittleEndian, uint32(offset))
		offset += len(e.data)
	}
	for _, e := range entries {
		ico.Write(e.data)
	}

	if err := os.WriteFile(*out, ico.Bytes(), 0o644); err != nil {
		fatal("写入 .ico 失败: %v", err)
	}
	fmt.Printf("已生成 %s（%d 个尺寸：%v，共 %d 字节）\n", *out, len(entries), iconSizes, ico.Len())
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
