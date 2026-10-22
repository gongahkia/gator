import { zipSync, strToU8 } from "fflate";
export const packageOffice = files => Buffer.from(zipSync(Object.fromEntries(Object.entries({ "[Content_Types].xml": '<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>', ...files }).map(([name, data]) => [name, typeof data === "string" ? strToU8(data) : data]))));
const rels = 'xmlns="http://schemas.openxmlformats.org/package/2006/relationships"';
export const wordFixture = () => packageOffice({
  "word/document.xml": '<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:rPr><w:b/></w:rPr><w:t xml:space="preserve">Hello </w:t></w:r><w:r><w:rPr><w:i/></w:rPr><w:t>world.</w:t></w:r></w:p><w:tbl><w:tr><w:tc><w:p><w:r><w:t>A table cell</w:t></w:r></w:p></w:tc></w:tr></w:tbl></w:body></w:document>',
  "word/styles.xml": '<styles>Preserve custom styles</styles>', "word/media/image.png": new Uint8Array([137,80,78,71,13,10,26,10]),
});
export const workbookFixture = () => packageOffice({
  "xl/workbook.xml": '<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Budget" sheetId="1" r:id="rId1"/></sheets></workbook>',
  "xl/_rels/workbook.xml.rels": `<Relationships ${rels}><Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>`,
  "xl/worksheets/sheet1.xml": '<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>Budget</t></is></c><c r="B1" s="2"><v>4</v></c><c r="C1"><f>B1*2</f><v>8</v></c></row></sheetData></worksheet>',
  "xl/styles.xml": '<styles>Keep workbook styles</styles>', "xl/charts/chart1.xml": '<chart>Keep the chart</chart>',
});
export const slidesFixture = () => packageOffice({
  "ppt/presentation.xml": '<p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><p:sldIdLst><p:sldId id="256" r:id="rId2"/></p:sldIdLst><p:sldSz cx="12192000" cy="6858000"/></p:presentation>',
  "ppt/_rels/presentation.xml.rels": `<Relationships ${rels}><Relationship Id="rId2" Target="slides/slide1.xml"/></Relationships>`,
  "ppt/slides/slide1.xml": '<p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><p:cSld><p:spTree><p:sp><p:nvSpPr><p:cNvPr id="2" name="Title"/></p:nvSpPr><p:spPr><a:xfrm><a:off x="100000" y="200000"/><a:ext cx="10000000" cy="1000000"/></a:xfrm></p:spPr><p:txBody><a:p><a:r><a:rPr b="1" sz="2800"/><a:t>Quarterly update</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld></p:sld>',
  "ppt/slideMasters/slideMaster1.xml": '<master>Preserve this master</master>', "ppt/media/logo.png": new Uint8Array([137,80,78,71,13,10,26,10]),
});
