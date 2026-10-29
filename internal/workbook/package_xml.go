package workbook

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

const sheetNS = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
const chartNS = "http://schemas.openxmlformats.org/drawingml/2006/chart"

type packageElement struct {
	name                      xml.Name
	attributes                []xml.Attr
	start, inside, close, end int
	parent                    *packageElement
	children                  []*packageElement
}
type packageEdit struct {
	start, end int
	contents   []byte
}

// Excelize's shared font/series structs serialize some children in an order
// rejected by Excel's schema. Normalize only these generated sequences; keep
// unrelated ZIP parts and individual XML child bytes unchanged.
func normalizePackageXML(contents []byte) ([]byte, error) {
	archive, err := zip.NewReader(bytes.NewReader(contents), int64(len(contents)))
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	for _, part := range archive.File {
		if part.Name != "xl/styles.xml" && !(strings.HasPrefix(part.Name, "xl/charts/chart") && strings.HasSuffix(part.Name, ".xml")) {
			if err := writer.Copy(part); err != nil {
				return nil, err
			}
			continue
		}
		reader, err := part.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(reader, 16*1024*1024+1))
		_ = reader.Close()
		if err != nil {
			return nil, err
		}
		if len(data) > 16*1024*1024 {
			return nil, errors.New("workbook style/chart part exceeds 16 MiB")
		}
		updated, err := normalizeSequences(data)
		if err != nil {
			return nil, fmt.Errorf("normalize %s: %w", part.Name, err)
		}
		if bytes.Equal(updated, data) {
			if err := writer.Copy(part); err != nil {
				return nil, err
			}
			continue
		}
		header := part.FileHeader
		entry, err := writer.CreateHeader(&header)
		if err != nil {
			return nil, err
		}
		if _, err := entry.Write(updated); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func normalizeSequences(data []byte) ([]byte, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var stack, elements []*packageElement
	for {
		before := int(decoder.InputOffset())
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch value := token.(type) {
		case xml.StartElement:
			node := &packageElement{name: value.Name, attributes: value.Copy().Attr, start: before, inside: int(decoder.InputOffset())}
			if len(stack) > 0 {
				node.parent = stack[len(stack)-1]
				node.parent.children = append(node.parent.children, node)
			}
			stack = append(stack, node)
			elements = append(elements, node)
		case xml.EndElement:
			node := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			node.close, node.end = before, int(decoder.InputOffset())
		}
	}
	var edits []packageEdit
	for _, node := range elements {
		if node.parent == nil {
			continue
		}
		var order, removable string
		if node.name == (xml.Name{Space: sheetNS, Local: "font"}) && node.parent.name == (xml.Name{Space: sheetNS, Local: "fonts"}) {
			order = "b i strike condense extend outline shadow u vertAlign sz color name family charset scheme"
		} else if node.name == (xml.Name{Space: chartNS, Local: "ser"}) && node.parent.name.Space == chartNS {
			switch node.parent.name.Local {
			case "barChart", "bar3DChart":
				order = "idx order tx spPr invertIfNegative pictureOptions dPt dLbls trendline errBars cat val shape extLst"
				removable = "smooth"
			case "lineChart", "line3DChart":
				order = "idx order tx spPr marker dPt dLbls trendline errBars cat val smooth extLst"
				removable = "invertIfNegative"
			case "pieChart", "pie3DChart":
				order = "idx order tx spPr explosion dPt dLbls cat val extLst"
				removable = "invertIfNegative smooth"
			}
		}
		if order == "" {
			continue
		}
		updated, err := orderedChildren(data, node, order, removable)
		if err != nil {
			return nil, err
		}
		if updated != nil && !bytes.Equal(updated, data[node.inside:node.close]) {
			edits = append(edits, packageEdit{node.inside, node.close, updated})
		}
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].start < edits[j].start })
	var output bytes.Buffer
	position := 0
	for _, edit := range edits {
		if edit.start < position {
			return nil, errors.New("overlapping workbook XML normalization")
		}
		output.Write(data[position:edit.start])
		output.Write(edit.contents)
		position = edit.end
	}
	output.Write(data[position:])
	return output.Bytes(), nil
}

func orderedChildren(data []byte, node *packageElement, order, removable string) ([]byte, error) {
	rank := map[string]int{}
	for i, name := range strings.Fields(order) {
		rank[name] = i
	}
	remove := map[string]bool{}
	for _, name := range strings.Fields(removable) {
		remove[name] = true
	}
	var children []*packageElement
	previous := node.inside
	for _, child := range node.children {
		// Unknown extensions or mixed content are retained, not guessed at.
		if child.name.Space != node.name.Space || len(bytes.TrimSpace(data[previous:child.start])) != 0 {
			return nil, nil
		}
		previous = child.end
		if remove[child.name.Local] {
			if len(child.attributes) != 1 || child.attributes[0].Name.Local != "val" || (child.attributes[0].Value != "0" && child.attributes[0].Value != "false") || len(child.children) > 0 {
				return nil, errors.New("unsupported non-default chart series property")
			}
			continue
		}
		if _, ok := rank[child.name.Local]; !ok {
			return nil, nil
		}
		children = append(children, child)
	}
	if len(bytes.TrimSpace(data[previous:node.close])) != 0 {
		return nil, nil
	}
	sort.SliceStable(children, func(i, j int) bool { return rank[children[i].name.Local] < rank[children[j].name.Local] })
	var result bytes.Buffer
	for _, child := range children {
		result.Write(data[child.start:child.end])
	}
	return result.Bytes(), nil
}
