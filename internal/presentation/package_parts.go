package presentation

import (
	"path"
	"strings"
)

const presentationNamespaces = `xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"`
const emptyShapeTree = `<p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/></p:spTree>`
const slideColorMap = `<p:clrMap bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2" accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" hlink="hlink" folHlink="folHlink"/>`
const relationshipsHeader = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`

func slideMasterXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><p:sldMaster ` + presentationNamespaces + `><p:cSld>` + emptyShapeTree + `</p:cSld>` + slideColorMap + `<p:sldLayoutIdLst><p:sldLayoutId id="2147483649" r:id="rId1"/></p:sldLayoutIdLst><p:txStyles><p:titleStyle/><p:bodyStyle/><p:otherStyle/></p:txStyles></p:sldMaster>`
}

func slideLayoutXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><p:sldLayout ` + presentationNamespaces + ` type="blank" preserve="1"><p:cSld name="Blank">` + emptyShapeTree + `</p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sldLayout>`
}

func masterRelationships() string {
	return relationshipsHeader + `<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideLayout" Target="../slideLayouts/slideLayout1.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/theme" Target="../theme/theme1.xml"/></Relationships>`
}

func layoutRelationships() string {
	return relationshipsHeader + `<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideMaster" Target="../slideMasters/slideMaster1.xml"/></Relationships>`
}

func notesMasterRelationships() string {
	return relationshipsHeader + `<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/theme" Target="../theme/theme1.xml"/></Relationships>`
}

func themeStyleLists() string {
	fill := `<a:solidFill><a:schemeClr val="phClr"/></a:solidFill>`
	line := `<a:ln w="9525" cap="flat" cmpd="sng" algn="ctr">` + fill + `<a:prstDash val="solid"/></a:ln>`
	return `<a:fillStyleLst>` + strings.Repeat(fill, 3) + `</a:fillStyleLst><a:lnStyleLst>` + strings.Repeat(line, 3) + `</a:lnStyleLst><a:effectStyleLst>` + strings.Repeat(`<a:effectStyle><a:effectLst/></a:effectStyle>`, 3) + `</a:effectStyleLst><a:bgFillStyleLst>` + strings.Repeat(fill, 3) + `</a:bgFillStyleLst>`
}

func (d *deck) existingLayout() string {
	for _, slide := range d.slides {
		relations := d.files[path.Join(path.Dir(slide.path), "_rels", path.Base(slide.path)+".rels")]
		for _, relation := range parseRelationships(relations.contents) {
			if !strings.HasSuffix(relation.Type, "/slideLayout") || (relation.TargetMode != "" && relation.TargetMode != "Internal") {
				continue
			}
			if target := resolveTarget(slide.path, relation.Target); target != "" && len(d.files[target].contents) > 0 {
				return target
			}
		}
	}
	return ""
}

func relativePartTarget(source, target string) string {
	from, to := strings.Split(path.Dir(source), "/"), strings.Split(target, "/")
	common := 0
	for common < len(from) && common < len(to) && from[common] == to[common] {
		common++
	}
	return strings.Repeat("../", len(from)-common) + strings.Join(to[common:], "/")
}
