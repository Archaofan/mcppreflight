// tools/peicon 解析 PE 资源目录，验证 exe 是否嵌入了应用图标。
//
// 用法：go run ./tools/peicon <exe路径>
package main

import (
	"debug/pe"
	"encoding/binary"
	"fmt"
	"os"
	"strings"
)

const (
	rtIcon      = 3  // RT_ICON：单个位图
	rtGroupIcon = 14 // RT_GROUP_ICON：图标组（Explorer/任务栏实际引用）
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "用法: go run ./tools/peicon <exe>")
		os.Exit(2)
	}
	f, err := pe.Open(os.Args[1])
	if err != nil {
		fatal("打开 PE 失败: %v", err)
	}
	defer f.Close()

	var rsrc []byte
	var rsrcVA uint32
	for _, s := range f.Sections {
		if strings.TrimRight(s.Name, "\x00") == ".rsrc" {
			rsrc, err = s.Data()
			if err != nil {
				fatal("读取 .rsrc 失败: %v", err)
			}
			rsrcVA = s.VirtualAddress
			break
		}
	}
	if rsrc == nil {
		fatal("未找到 .rsrc 节：exe 没有嵌入任何资源")
	}

	// 遍历资源目录（最多三层：类型 -> 名称/ID -> 语言）
	var walk func(off uint32, depth int, typ uint32, name uint32)
	groups := map[uint32][]byte{} // group id -> raw GRPICONDIR
	iconCount := 0
	walk = func(off uint32, depth int, typ uint32, name uint32) {
		if int(off+16) > len(rsrc) {
			return
		}
		nNamed := binary.LittleEndian.Uint16(rsrc[off+12:])
		nID := binary.LittleEndian.Uint16(rsrc[off+14:])
		total := int(nNamed) + int(nID)
		for i := 0; i < total; i++ {
			e := int(off+16) + i*8
			if e+8 > len(rsrc) {
				return
			}
			entryName := binary.LittleEndian.Uint32(rsrc[e:])
			entryOff := binary.LittleEndian.Uint32(rsrc[e+4:])
			isDir := entryOff&0x80000000 != 0
			childOff := entryOff & 0x7FFFFFFF
			id := entryName & 0x7FFFFFFF
			if depth == 0 {
				typ = id
				name = 0
			} else if depth == 1 {
				name = id
			}
			if isDir {
				walk(childOff, depth+1, typ, name)
				continue
			}
			// 叶子：IMAGE_RESOURCE_DATA_ENTRY
			if int(childOff+16) > len(rsrc) {
				continue
			}
			dataRVA := binary.LittleEndian.Uint32(rsrc[childOff:])
			size := binary.LittleEndian.Uint32(rsrc[childOff+4:])
			fileOff := int64(dataRVA) - int64(rsrcVA)
			if fileOff < 0 || fileOff+int64(size) > int64(len(rsrc)) {
				continue
			}
			raw := rsrc[fileOff : fileOff+int64(size)]
			switch typ {
			case rtGroupIcon:
				groups[name] = raw
			case rtIcon:
				iconCount++
			}
		}
	}
	walk(0, 0, 0, 0)

	fmt.Printf("RT_GROUP_ICON 数量: %d\n", len(groups))
	fmt.Printf("RT_ICON 位图数量: %d\n", iconCount)
	if len(groups) == 0 {
		fmt.Println("结论: exe 未嵌入应用图标")
		os.Exit(1)
	}
	for id, raw := range groups {
		if len(raw) < 6 {
			continue
		}
		count := binary.LittleEndian.Uint16(raw[4:])
		var sizes []string
		for i := 0; i < int(count) && 6+i*14+14 <= len(raw); i++ {
			e := 6 + i*14
			w, h := int(raw[e]), int(raw[e+1])
			if w == 0 {
				w = 256
			}
			if h == 0 {
				h = 256
			}
			sizes = append(sizes, fmt.Sprintf("%dx%d", w, h))
		}
		fmt.Printf("图标组 ID=%d：%d 个尺寸 [%s]\n", id, count, strings.Join(sizes, " "))
	}
	fmt.Println("结论: exe 已成功嵌入应用图标")
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
