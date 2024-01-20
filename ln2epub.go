// Scrape LN TL sites and convert to Epub.
// Licensed under BSD 2-Clause License.
package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"github.com/anaskhan96/soup"
	// "golang.org/x/net/proxy"
	"io/ioutil"
	"net/http"
	nurl "net/url"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// * Epub

// Epub is a zip file with the following required files:
// . mimetype - which should be the first and contains the mimetype of
//   the file---application/epub+zip.
// . META-INF/container.xml - this file tells where the "root file" is.
//   Root file is where the rest of the files listed below are stored.
//   Usually the location is "OEBPS".
// . OEBPS/content.opf - this file lists all the files used by the epub.
// . toc.ncx - this has the order of the files to be opened aka Table
//   of Contents.
// The epub wikipedia page has a very nice summary: https://en.wikipedia.org/wiki/EPUB#Version_2.0.1
// Opening the epub archive yourself will also give you a good idea of
// what needs to be done.

type EpubFile struct {
	// Title is the title of the chapter if it is a xhtml file.
	Title string

	// Content is the content of the file.
	Content []byte

	// Filename of file.
	Filename string

	// Mimetype of the file.
	Mimetype string

	// `Id' attribute of the file.
	Id string
}

func EpubstripOebpsPrefix(filename string) string {
	return strings.TrimPrefix(filename, "OEBPS/")
}

// Return the file contents of the content.opf file for the series.
// AUTHOR is the author of the series, TITLE is the name of the
// series, IDENTIFIER is the value of unique-identifier for the
// series, FILES is a list of EpubFile.
// Filenames are stripped off "OEBPS/" prefix.
// If the epub file Id is "cover", then it is taken as the cover image
// page and treated specially.
// The unique identifier used will always be "BookId".
func EpubContentOpf(author, identifier, title string, files []EpubFile) []byte {
	var content bytes.Buffer
	var manifest strings.Builder
	var cover EpubFile
	var coverImg EpubFile

	// Do the manifest first and figure out if there's a cover
	// image.
	manifest.WriteString("<manifest>")
	for _, i := range files {
		if i.Id == "cover" {
			cover = i
		}
		if i.Id == "cover-image" {
			coverImg = i
		}
		manifest.WriteString("\n<item id='")
		manifest.WriteString(i.Id)
		manifest.WriteString("' href='")
		manifest.WriteString(EpubstripOebpsPrefix(i.Filename))
		manifest.WriteString("' media-type='")
		manifest.WriteString(i.Mimetype)
		manifest.WriteString("'")
		if i.Id == "cover-image" {
			manifest.WriteString(" properties='cover-image'")
		}
		manifest.WriteString(" />")
	}
	manifest.WriteString("\n<item id='ncx' href='toc.ncx' media-type='application/x-dtbncx+xml'/>\n</manifest>\n")

	// Header.
	content.WriteString(`<?xml version="1.0" encoding="utf-8"?>
<package version="2.0" unique-identifier="BookId" xmlns="http://www.idpf.org/2007/opf">`)

	// First do the metadata section.
	content.WriteString(`<metadata xmlns:dc="http://purl.org/dc/elements/1.1/"  xmlns:opf="http://www.idpf.org/2007/opf">
`)
	content.WriteString("<dc:creator>")
	content.WriteString(author)
	content.WriteString("</dc:creator>\n")
	content.WriteString("<dc:identifier id=\"BookId\">")
	content.WriteString(identifier)
	content.WriteString("</dc:identifier>\n")
	content.WriteString("<dc:language>en</dc:language>\n")
	content.WriteString("<dc:title>")
	content.WriteString(title)
	content.WriteString("</dc:title>\n")
	content.WriteString(`<dc:date opf:event="modification" xmlns:opf="http://www.idpf.org/2007/opf">`)
	content.WriteString(time.Now().Format("2022-01-18"))
	content.WriteString("</dc:date>\n")
	if coverImg.Id == "cover-image" {
		content.WriteString("<meta name='cover' content='")
		// I can't tell what exactly this should be!
		// Different epubs use different value here, but
		// thankfully the exact value does not matter.
		content.WriteString(coverImg.Id)
		content.WriteString("' />\n")
	}
	content.WriteString("</metadata>\n\n")

	// Manifest section.
	content.WriteString(manifest.String())

	// Spine section.
	content.WriteString("\n<spine toc='ncx'>")
	for _, i := range files {
		if i.Mimetype != "application/xhtml+xml" {
			continue
		}
		content.WriteString("\n<itemref idref='")
		content.WriteString(i.Id)
		content.WriteString("'/>")
	}
	content.WriteString("\n</spine>\n")

	if cover.Id == "cover" {
		content.WriteString(`<guide><reference type="cover" title="Cover" href="`)
		content.WriteString(EpubstripOebpsPrefix(cover.Filename))
		content.WriteString(`" /></guide>`)
	}
	content.WriteString("</package>\n")
	return content.Bytes()
}

// Return the file contents of the toc.ncx file for the series.
// Arguments have the same meaning as for EpubContentOpf.
// FILES with a mimetype other than xhtml, and cover image xhtml file
// are ignored.
// Filenames are stripped off "OEBPS/" prefix.
func EpubTocNcx(author, identifer, title string, files []EpubFile) []byte {
	var content bytes.Buffer

	// Header.
	content.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE ncx PUBLIC "-//NISO//DTD ncx 2005-1//EN" "http://www.daisy.org/z3986/2005/ncx-2005-1.dtd">

<ncx version="2005-1" xml:lang="en" xmlns="http://www.daisy.org/z3986/2005/ncx/">
  <head>
    <meta name="dtb:uid" content="`)
	content.WriteString(identifer)
	content.WriteString(`"/>
    <meta name="dtb:depth" content="1"/>
    <meta name="dtb:totalPageCount" content="0"/>
    <meta name="dtb:maxPageNumber" content="0"/>
  </head>

  <docTitle><text>`)
	content.WriteString(title)
	content.WriteString(`</text></docTitle>
  <docAuthor><text>`)
	content.WriteString(author)
	content.WriteString(`</text></docAuthor>
  <navMap>`)

	n := 1
	// Now for the nested structure.
	for _, i := range files {
		// Non-xhtml files.
		if i.Mimetype != "application/xhtml+xml" {
			continue
		}
		// Ignore cover page.
		if i.Id == "cover" {
			continue
		}
		content.WriteString("\n<navPoint id='")
		content.WriteString(i.Id)
		content.WriteString("' playOrder='")
		content.WriteString(strconv.Itoa(n))
		content.WriteString("'>\n")

		content.WriteString("<navLabel><text>")
		content.WriteString(i.Title)
		content.WriteString("</text></navLabel>\n")
		content.WriteString("<content src='")
		content.WriteString(EpubstripOebpsPrefix(i.Filename))
		content.WriteString("' />\n</navPoint>")
		n += 1
	}

	content.WriteString("\n</navMap>\n</ncx>\n")
	return content.Bytes()
}

// Return the file contents of the container.xml file.
func EpubContainerXml() []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
    <rootfiles>
        <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
   </rootfiles>
</container>
`)
}

// Return the file contents of the mimetype file.
func EpubMimetype() []byte {
	return []byte("application/epub+zip\n")
}

// Return the preamble for xhtml content files for chapter with TITLE.
func EpubContentPreamble(title string) string {
	return `<?xml version="1.0" encoding="UTF-8" ?>
<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.1//EN" "http://www.w3.org/TR/xhtml11/DTD/xhtml11.dtd">
<html xmlns="http://www.w3.org/1999/xhtml" xml:lang="en">
  <head>
    <meta http-equiv="Content-Type" content="application/xhtml+xml; charset=utf-8" />
    <title>` + title + `</title>
  </head>
  <body>`
}

// Return the ending part for xhtml content files.
func EpubContentEnd() string {
	return `</body>
</html>
`
}

// Create an .epub file with filename FILENAME.
func EpubCreateFile(filename string, files []EpubFile) {
	epubFile, err := os.Create(filename)
	if err != nil {
		panic(err)
	}
	defer epubFile.Close()

	w := zip.NewWriter(epubFile)
	defer w.Close()

	for _, file := range files {
		f, err := w.Create(file.Filename)
		if err != nil {
			panic(err)
		}
		_, err = f.Write(file.Content)
		if err != nil {
			panic(err)
		}
	}
}

// Add the extra manadatory epub files to FILES.
// Rest of the arguments are passed as-is to EpubContentOpf and
// friends.
func EpubAddExtra(author, identifier, title string, files []EpubFile) []EpubFile {
	contentOpf := EpubContentOpf(author, identifier, title, files)
	tocNcx := EpubTocNcx(author, identifier, title, files)

	files = append(files,
		[]EpubFile{
			{
				Content:  contentOpf,
				Filename: "OEBPS/content.opf",
			},
			{
				Content:  tocNcx,
				Filename: "OEBPS/toc.ncx",
			},
			{
				Content:  EpubContainerXml(),
				Filename: "META-INF/container.xml",
			},
		}...)
	files = append([]EpubFile{
		{
			Content:  EpubMimetype(),
			Filename: "mimetype",
		},
	},
		files...)
	return files
}

// * Fetch helpers

// Headers to use when making HTTP requests.
var RHEADERS map[string][]string = map[string][]string{
	"User-Agent": {"Chrome/96.0.4664.110"},
}

// Make a GET/POST request for URL.
func fetch(url string, postform nurl.Values) ([]byte, error) {
	var req *http.Request
	if postform == nil {
		req, _ = http.NewRequest("GET", url, nil)
		req.Header = RHEADERS
	} else {
		req, _ = http.NewRequest("POST", url,
			strings.NewReader(postform.Encode()))
		req.Header = RHEADERS
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	}

	// proxyURL, err := nurl.Parse("socks5://127.0.0.1:9050")
	// dialer, err := proxy.FromURL(proxyURL, proxy.Direct)
	// if err != nil {
	// 	panic(err)
	// }
	// transport := &http.Transport{Dial: dialer.Dial}
	// client := &http.Client{Transport: transport}
	client := &http.Client{}
	resp, err := client.Do(req)
	client.CloseIdleConnections()
	if err != nil {
		return []byte(""), err
	}
	body, _ := ioutil.ReadAll(resp.Body)

	return body, nil
}

// Make a GET request for URL.
// The response body and error, if any, are returned.
func Request(url string) (string, error) {
	b, e := fetch(url, nil)
	return string(b), e
}

// Make a POST rqeuest for URL with form POSTFORM.
func PostForm(url string, postform nurl.Values) (string, error) {
	b, e := fetch(url, postform)
	return string(b), e
}

// Fetch the image from url URL.
// Return the image file contents, image mimetype.
func FetchImage(url string) ([]byte, string) {
	img, _ := fetch(url, nil)
	return img, http.DetectContentType(img)
}

// Fetched images with key as URL.
var ImageCache = make(map[string]EpubFile)

// Return image located at URL if cached, or make new one.
// N is the chapter name, and IMGCOUNTER is the number assigned to
// image.  These are used if it is not found in `ImageCache'.
// The second return value is the new IMGCOUNTER value.
func FetchImageCached(url string, n, imgCounter int) (EpubFile, int) {
	var ifile EpubFile
	var ok bool
	if ifile, ok = ImageCache[url]; !ok {
		imgId := fmt.Sprintf("Img%d_Ch%d", imgCounter, n)
		img, mimetype := FetchImage(url)
		ifile = EpubFile{
			Id: imgId,
			Filename: "OEBPS/Images/" + imgId,
			Mimetype: mimetype,
			Content: img,
		}
		ImageCache[url] = ifile
		imgCounter += 1
	}
	return ifile, imgCounter
}


// * Common routines

// Return an approriate epub filename for URL.
// TODO: Should include an optional part to indicate how many chapters
// have been fetched.
func EpubFileName(url string) string {
	return path.Base(strings.TrimSuffix(url, "/")) + ".epub"
}

// From https://github.com/anaskhan96/soup/issues/63.
func SoupFindParent(r soup.Root, tagName string) soup.Root {
	parent := r.Pointer.Parent
	if parent == nil {
		return soup.Root{Pointer: parent}
	}
	rParent := soup.Root{Pointer: parent, NodeValue: parent.Data}
	if strings.ToLower(parent.Data) == strings.ToLower(tagName) {
		return rParent
	}
	return SoupFindParent(rParent, tagName)
}

// Fetch images in IMGS and replace HTML with local `src' value.
// IMGCOUNTER is the number of image in this chapter, N is the chapter
// number, and ADDTO is the list to add EpubFile struct of the fetched
// image to.
// New value of HTML, IMGCOUNTER, ADDTO are returned.
func ReplaceImgTags(html string, imgs []soup.Root, imgCounter, n int, addto []EpubFile) (string, int, []EpubFile) {
	if len(imgs) == 0 {
		return html, imgCounter, addto
	}
	var imgBuf strings.Builder

	for _, img := range imgs {
		ifile, ic := FetchImageCached(img.Attrs()["src"], n, imgCounter)
		// New image.
		if ic != imgCounter {
			addto = append(addto, ifile)
			imgCounter = ic
		}
		imgBuf.WriteString("<img src='../")
		imgBuf.WriteString(EpubstripOebpsPrefix(ifile.Filename))
		imgBuf.WriteString("' ")
		for _, a := range []string{"width", "alt", "height"} {
			if at, ok := img.Attrs()[a]; ok {
				imgBuf.WriteString(a)
				imgBuf.WriteString("='")
				imgBuf.WriteString(at)
				imgBuf.WriteString("' ")
			}
		}
		imgBuf.WriteString("/>")

		html = strings.ReplaceAll(html,
			img.HTML(),
			imgBuf.String())
	}

	return html, imgCounter, addto
}

// Fetch and add cover with URL URL to FILES.
func AddCoverImage(url string, files []EpubFile) []EpubFile {
	cover, mimetype := FetchImage(url)
	c := EpubFile{
		Id: "cover-image",
		Filename: "OEBPS/Images/cover",
		Mimetype: mimetype,
		Content: cover}
	files = append(files, c)
	ImageCache[url] = c

	var cfile bytes.Buffer
	cfile.WriteString(EpubContentPreamble("cover"))
	cfile.WriteString("<img src='../Images/cover' />")
	cfile.WriteString(EpubContentEnd())

	files = append(files,
		EpubFile{
			Title: "Cover",
			Id: "cover",
			Filename: "OEBPS/Text/Cover.xhtml",
			Mimetype: "application/xhtml+xml",
			Content: cfile.Bytes(),
		})
	return files
}

// Return the tag name of S or "" if nil.
func SoupTag(s soup.Root) string {
	if s.Pointer != nil {
		return s.Pointer.Data
	} else {
		return ""
	}
}

// * Soafp

// TODO: Get description of the series and cover image.

var SoapcleanChNameRe = regexp.MustCompile(`([[:space:]]+:[[:space:]]+)`)

// Return the series title with URL URL.
func SoafpSeriesTitle(url string) string {
	h, err := Request(url)
	if err != nil {
		panic(err)
	}

	s := soup.HTMLParse(h)
	return s.Find("h1", "class", "entry-title").Text()
}

// Normalise CHAPTERNAME to not contain punctuation mistakes.
func SoafpcleanChapterName(chaptername string) string {
	return SoapcleanChNameRe.ReplaceAllString(chaptername, ": ")
}

// Return a list of [ URL, CHAPTERNAME ] for the series in URL.
func SoafpChapters(url string) [][]string {
	h, err := Request(url)
	if err != nil {
		panic(err)
	}

	var chapters [][]string

	s := soup.HTMLParse(h)
	as := s.Find("ul", "class", "lcp_catlist").FindAll("a")

	for _, a := range as {
		chapters = append(chapters, []string{
			a.Attrs()["href"],
			SoafpcleanChapterName(a.Text()),
		})
	}

	return chapters
}

// Return true if attribute VALUE contains STR.
func HtmlValueContains(str, value string) bool {
	if strings.Contains(value, " ") {
		vs := strings.Split(value, " ")
		for _, v := range vs {
			if v == str {
				return true
			}
		}
	} else {
		return value == str
	}
	return false
}

func SoafpisAd(value string) bool {
	return HtmlValueContains("code-block", value)
}

func SoafpisEnd(value string) bool {
	return HtmlValueContains("wp-block-buttons", value) ||
		HtmlValueContains("sd-like", value) ||
		HtmlValueContains("daddy", value)
}

func SoafpisSettingsButton(value string) bool {
	return HtmlValueContains("pre-bar", value)
}

func SoafpisImg(value string) bool {
	return HtmlValueContains("wp-block-image", value)
}

var SoafpchapterNoPrefix = regexp.MustCompile(`[Cc]hapter [0-9]+: `)

// Return CHAPTERNAME without the initial chapter numbering.
func SoafpstripChapterNo(chaptername string) string {
	return SoafpchapterNoPrefix.ReplaceAllString(chaptername, "")
}

// Return content for chapter URL with CHAPTERNAME, chapter no. N.
// If the chapter has extra files, then it is returned as the second
// argument.
// TODO: Replace all images like in the rest.
func SoafpChapter(url, chaptername string, n int) ([]byte, []EpubFile) {
	var ret bytes.Buffer
	var extra []EpubFile

	h, err := Request(url)
	if err != nil {
		panic(err)
	}

	// Write the preamble.
	ret.WriteString(EpubContentPreamble(SoafpstripChapterNo(chaptername)))
	ret.WriteString("\n<h1>")
	ret.WriteString(chaptername)
	ret.WriteString("</h1>")

	s := soup.HTMLParse(h)
	divChildren := s.Find("div", "class", "entry-content").Children()

	// We want to skip the ad containers and <span> tags, and end
	// when we encounter a <hr/> tag with approriate class.
	ret.WriteString("<div class='entry-content'>\n")
	imgCounter := 0
	for _, c := range divChildren {
		if c.Pointer.Data == "div" && SoafpisAd(c.Attrs()["class"]) {
			continue
		}

		if c.Pointer.Data == "div" && SoafpisSettingsButton(c.Attrs()["class"]) {
			continue
		}

		if c.Pointer.Data == "div" && SoafpisEnd(c.Attrs()["class"]) {
			break
		}

		if c.Pointer.Data == "div" && SoafpisImg(c.Attrs()["class"]) {
			imgAttrs := c.Find("img").Attrs()
			ifile, ic := FetchImageCached(imgAttrs["href"], n, imgCounter)
			// New image.
			if ic != imgCounter {
				imgCounter = ic
				extra = append(extra, ifile)
			}
			imgFileName := EpubstripOebpsPrefix(ifile.Filename)

			ret.WriteString("<img src='../")
			ret.WriteString(imgFileName)
			ret.WriteString("' ")
			for _, a := range []string{"width", "height", "alt"} {
				if w, ok := imgAttrs[a]; ok {
					ret.WriteString(fmt.Sprintf("%s='%s' ", a, w))
				}
			}
			ret.WriteString("/>\n")

			continue
		}

		ht := c.HTML()

		// if sps := c.FindAll("span", "class", "has-inline-color"); sps != nil {
		// 	for _, sp := range sps {
		// 		ht = strings.ReplaceAll(ht, sp.HTML(), "")
		// 	}
		// }

		ret.WriteString(ht)
	}
	ret.WriteString("</div>")
	ret.WriteString(EpubContentEnd())

	return ret.Bytes(), extra
}

// Return the list of EpubFile for the series in URL.
func SoafpEpubFiles(url string) map[string][]EpubFile {
	var files []EpubFile
	chs := SoafpChapters(url)

	for n, ch := range chs {
		fmt.Println("Fetching chapter", ch[0])
		content, extra := SoafpChapter(ch[0], ch[1], n)
		files = append(
			files,
			EpubFile{
				Title:    ch[1],
				Id:       "Chapter" + strconv.Itoa(n+1),
				Content:  content,
				Filename: "OEBPS/Text/Chapter" + strconv.Itoa(n+1) + ".xhtml",
				Mimetype: "application/xhtml+xml",
			})
		files = append(files, extra...)
	}

	title := SoafpSeriesTitle(url)
	return map[string][]EpubFile{
		url: EpubAddExtra("", title, title, files),
	}
}

// * Shalvation Translations

// Get the author of the series with TOC page soup SP.
func ShalvationAuthor(sp soup.Root) string {
	// The first <string> tag is for the author.
	strong := sp.Find("div", "class", "entry-content").Find("strong")
	// Needs to be HTML().  Text() and FullText() both return an
	// empty string.
	return strong.FindNextSibling().HTML()
}

// Return the cover image URL of the series with TOC page soup SP.
func ShalvationCoverImg(sp soup.Root) string {
	return sp.Find("div", "class", "entry-content").Find("img").Attrs()["src"]
}

// Return the synposis HTML for the series with TOC page soup SP.
// An empty string is returned if there is none.
func ShalvationSynopsis(sp soup.Root) string {
	var h strings.Builder
	synp := sp.Find("div", "class", "entry-content").Find("h4")
	// First check if it is a synposis heading.
	if strings.ToLower(strings.TrimSpace(synp.Text())) == "synposis" {
		return ""
	}

	h.WriteString(synp.HTML())
	// The synposis lasts till a <hr> tag.
	for s := synp.FindNextSibling(); s.Pointer != nil &&
		s.Pointer.Data != "hr"; s = s.FindNextSibling() {
		h.WriteString(s.HTML())
	}

	return h.String()
}

func ShalvationendOfVol(s soup.Root) bool {
	return s.Pointer.Data == "div" &&
		strings.HasPrefix(s.Attrs()["id"], "atatags-")
}

func ShalvationvolLinks(sp soup.Root) [][]string {
	var links [][]string

	for s := sp.FindNextSibling(); s.Pointer != nil &&
		s.Pointer.Data != "hr" && !ShalvationendOfVol(s);
	s = s.FindNextSibling() {
		if a := s.Find("a"); a.Pointer != nil {
			if strings.Contains(a.Attrs()["href"], "drive.google.com") {
				continue
			}
			links = append(links, []string{a.FullText(), a.Attrs()["href"]})
		} else if img := s.Find("img"); img.Pointer != nil {
			links = append(links, []string{"cover", img.Attrs()["src"]})
		}
	}
	if len(links) == 1 {
		return nil
	}
	return links
}

// Return links for each chapter in every volume for TOC page soup SP.
// A map is written where key is the name of the volume, and value is
// a list of [ CHAPTERNAME, CHAPTERURL ].
func ShalvationVols(sp soup.Root) map[string][][]string {
	vols := make(map[string][][]string)
	div := sp.Find("div", "class", "entry-content")

	for _, h4 := range div.FindAll("h4") {
		if strings.ToLower(h4.Text()) == "synposis" {
			continue
		}
		vol := ShalvationvolLinks(h4)
		if vol == nil {
			continue
		}
		vols[h4.Text()] = vol
	}

	return vols
}

var ShalvationChapterNoRe = regexp.MustCompile(`Chapter [0-9]+ – `)

func ShalvationstripChapterNo(chaptername string) string {
	return ShalvationChapterNoRe.ReplaceAllString(chaptername, "")
}

// Return content for chapter URL with CHAPTERNAME, chapter no. N.
// If the chapter has images, then it is returned as the second
// argument.
func ShalvationChapter(url, chaptername string, n int) ([]byte, []EpubFile) {
	var content bytes.Buffer
	var extra []EpubFile

	h, err := Request(url)
	if err != nil {
		panic(err)
	}

	chaptername = ShalvationstripChapterNo(chaptername)
	content.WriteString(EpubContentPreamble(chaptername))

	sp := soup.HTMLParse(h)
	div := sp.Find("div", "class", "entry-content")
	firstHrSkipped := false
	imgCounter := 1
	for _, c := range div.Children() {
		if c.Pointer.Data == "hr" && !firstHrSkipped {
			firstHrSkipped = true
		} else if c.Pointer.Data == "h4" &&
			strings.Contains(c.Text(), chaptername) {
			content.WriteString("<h1>")
			content.WriteString(chaptername)
			content.WriteString("</h1>\n")
		} else if imgs := c.FindAll("img"); len(imgs) != 0 {
			var html string
			html, imgCounter, extra = ReplaceImgTags(
				c.HTML(), imgs,
				imgCounter, n,
				extra)
			content.WriteString(html)
		} else if u := c.Find("span"); u.Pointer != nil &&
			(strings.HasPrefix(u.Attrs()["id"], "more-") ||
				strings.HasPrefix(u.Attrs()["style"], "color:#4c4c48;")) {
			continue
		} else if a := c.Find("a"); a.Pointer != nil &&
			(strings.Contains(a.Text(), " CHAPTER") ||
				strings.Contains(a.Text(), "VOLUME")) {
			// TODO: Condition to break the chapters is
			// extremely adhoc, need to fix it.
			break
		} else {
			content.WriteString(c.HTML())
		}
	}

	content.WriteString(EpubContentEnd())

	return content.Bytes(), extra
}

func ShalvationprepVol(links [][]string) []EpubFile {
	var files []EpubFile
	n := 1
	for _, l := range links {
		switch l[0] {
		case "cover":
			fmt.Println("Fetching cover page...")
			files = AddCoverImage(l[1], files)
		case "Illustrations":
			fmt.Println("Fetching illustrations...")
			f, e := ShalvationChapter(l[1], "Illustrations", -1)
			files = append(files, e...)
			files = append(files,
				EpubFile{
					Title: "Illustrations",
					Id: "illustrations",
					Filename: "OEBPS/Text/Illustrations.xhtml",
					Mimetype: "application/xhtml+xml",
					Content: f,
				})
		default:
			fmt.Println("Fetching", l[0])
			cid := "Chapter" + strconv.Itoa(n)
			f, e := ShalvationChapter(l[1], l[0], n)
			files = append(files, e...)
			files = append(files,
				EpubFile{
					Title: l[0],
					Id: cid,
					Filename: "OEBPS/Text/" + cid + ".xhtml",
					Mimetype: "application/xhtml+xml",
					Content: f,
				})
			n += 1
		}
	}
	return files
}

var ShalvationtitleRe = regexp.MustCompile(` Table[\s\p{Zs}][oO]f[\s\p{Zs}]Contents$`)

// Return the title for the series TOC soup SP.
func ShalvationTitle(sp soup.Root) string {
	title := sp.Find("h1", "class", "entry-title")
	return ShalvationtitleRe.ReplaceAllString(title.Text(), "")
}

// Return the epub files for series TOC URL.
func ShalvationEpubFiles(url string) map[string][]EpubFile {
	h, err := Request(url)
	if err != nil {
		panic(err)
	}

	ret := make(map[string][]EpubFile)

	sp := soup.HTMLParse(h)
	vols := ShalvationVols(sp)
	author := ShalvationAuthor(sp)
	title := ShalvationTitle(sp)

	for vol, links := range vols {
		files := ShalvationprepVol(links)
		files = append(files,
			EpubAddExtra(author,
				strings.ReplaceAll(title + vol, " ", "-"),
				title + " " + vol,
				files)...)
		ret["/" + strings.ReplaceAll(vol, " ", "-")] = files
	}
	return ret
}

// * Baka-tsuki (Hyouka)

// Return link to all the volumes in URL URL.
// Returned is a list of [ TITLE, LINK ] where TITLE is the title of
// the volume, and LINK is the link to the volume full text.
func BakatsukiVolumes(url string) [][]string {
	var ret [][]string
	h, err := Request(url)
	if err != nil {
		panic(err)
	}
	soup := soup.HTMLParse(h)

	for _, i := range soup.FindAll("span", "class", "mw-headline") {
		if a := i.Find("a"); a.Pointer != nil && a.Text() == "Full Text" {
			ret = append(ret,
				[]string{
					// -2 to remove " (" from the end.
					i.Text()[:len(i.Text())-2],
					"https://www.baka-tsuki.org" + i.Find("a").Attrs()["href"],
				})
		}
	}
	return ret
}

// Return the EpubFiles for volume with URL URL.
// TODO: It would be nice to have working TL note links.
func BakatsukiPrepVol(url string) []EpubFile {
	h, err := Request(url)
	if err != nil {
		panic(err)
	}
	soup := soup.HTMLParse(h)

	var files []EpubFile

	spans := soup.FindAll("h2")
	chi := 1
	for sn, h2 := range spans {
		var chapter bytes.Buffer

		i := h2.Find("span", "class", "mw-headline")
		if i.Pointer == nil {
			continue
		}
		chid := "Chapter" + strconv.Itoa(chi)
		imgCounter := 1

		chapter.WriteString(EpubContentPreamble(i.Text()))
		chapter.WriteString("<h1>")
		chapter.WriteString(i.Text())
		chapter.WriteString("</h1>\n")

		for s := h2.FindNextSibling(); s.Pointer != nil &&
			s.Pointer.Data != "h2"; s = s.FindNextSibling() {
			if sn == len(spans)-1 && s.Pointer.Data == "table" {
				break
			}
			str := s.HTML()
			if edit := s.Find("span", "class", "mw-editsection"); edit.Pointer != nil {
				str = strings.ReplaceAll(str, edit.HTML(), "")
			}
			str, imgCounter, files = ReplaceImgTags(str, s.FindAll("img"),
				imgCounter, chi, files)
			chapter.WriteString(str)
		}

		chapter.WriteString(EpubContentEnd())
		files = append(files,
			EpubFile{
				Title: i.Text(),
				Content: chapter.Bytes(),
				Filename: "OEBPS/Text/" + chid + ".xhtml",
				Mimetype: "application/xhtml+xml",
				Id: chid,
			})
		chi += 1
		chapter.Reset()
	}

	return files
}

// Return EpubFiles for each volume in series URL URL.
func BakatsukiEpubFiles(url string) map[string][]EpubFile {
	ret := make(map[string][]EpubFile)
	vols := BakatsukiVolumes(url)

	for _, vol := range vols {
		fmt.Println("Fetching", vol[1])
		chps := BakatsukiPrepVol(vol[1])
		v := "/" + strings.ReplaceAll(vol[0], "/", "∕")
		v = strings.ReplaceAll(v, " ", "_")
		ret[v] = EpubAddExtra("Baka-Tsuki TL",
				strings.ReplaceAll(vol[0], " ", "-"),
			vol[0], chps)
	}

	return ret
}

// * Travis Translations
// Fetch chapter links from series soup SUP.
// A list of [ CHAPTER-NAME, URL ] is returned.
func TravisChapters(sup soup.Root) [][]string {
	var div soup.Root
	// For some reason sup.Find() does not work!
	for  _, div =  range sup.FindAll("div") {
		if div.Attrs()["x-show"] == "tab === 'toc'" {
			break
		}
	}

	var chapter [][]string
	for _, li := range div.Find("ul").Children() {
		if li.Pointer.Data != "li" {
			continue
		}
		a := li.Find("a")
		if a.Pointer == nil {
			continue
		}
		chapter = append(chapter,
			[]string{
				a.Find("span").FullText(),
				a.Attrs()["href"]})
	}

	return chapter
}

// Return content for URL with name CHAPTERNAME, chapter no. N.
// If any extra files are to be attached, then it is returned as the
// second item.
func TravisChapter(url, chaptername string, n int) ([]byte, []EpubFile) {
	h, err := Request(url)
	if err != nil {
		panic(err)
	}

	var content bytes.Buffer
	var extra []EpubFile
	content.WriteString(EpubContentPreamble(chaptername))

	sup := soup.HTMLParse(h)
	div := sup.Find("div", "class", "reader-content").Find("p")
	start := false
	imgCounter := 1
	for p := div; p.Pointer != nil; p = p.FindNextSibling() {
		if p.Pointer.Data == "p" &&
			strings.HasPrefix(p.FullText(), "Edited by") {
			start = true
			p = p.FindNextSibling() // Skip <hr/>
		}
		if p.Pointer.Data == "p" &&
			p.Attrs()["style"] == "text-align: center;" &&
			strings.HasPrefix(p.FullText(), "Chapter ") {
			content.WriteString("<h1>")
			content.WriteString(p.FullText())
			content.WriteString("</h1>\n")
		}
		if p.Pointer.Data == "hr" {
			next := p.FindNextSibling().FindNextSibling()
			if next.Pointer.Data == "p" &&
				strings.HasPrefix(next.FullText(), "Read ") {
				break
			}
		}
		if !start {
			continue
		}
		var html string
		html, imgCounter, extra = ReplaceImgTags(p.HTML(), p.FindAll("img"),
			imgCounter, n, extra)
		content.WriteString(html)
	}

	return content.Bytes(), extra
}

// Return the series title for series soup SUP.
func TravisSeriesTitle(sup soup.Root) string {
	div := sup.Find("div", "id", "series-header")
	if div.Pointer == nil {
		return "***UNKNOWN***"
	}
	return div.Find("h1", "id", "heading").FullText()
}

// Return the EPub files for the series with URL URL.
func TravisEpubFiles(url string) map[string][]EpubFile {
	h, err := Request(url)
	if err != nil {
		panic(err)
	}
	sup := soup.HTMLParse(h)

	chapters := TravisChapters(sup)
	seriesTitle := TravisSeriesTitle(sup)

	var files []EpubFile
	for n, ch := range chapters {
		fmt.Println("Fetching", ch[1])
		content, extra := TravisChapter(ch[1], ch[0], n+1)
		cid := "Chapter" + strconv.Itoa(n+1)
		files = append(files,
			EpubFile{
				Title: ch[0],
				Id: cid,
				Filename: "OEBPS/Text/" + cid + ".xhtml",
				Mimetype: "application/xhtml+xml",
				Content: content,
			})
		files = append(files, extra...)
	}
	return map[string][]EpubFile{
		seriesTitle: EpubAddExtra(
			"Travis Translations",
			url, seriesTitle,
			files),
	}
}

// * Kequeen TLs
// Return list of chapter names in volume soup SUP.
// A list of [ NAME, CURL ] where NAME is chapter name with URL CURL.
// The cover link is returned with NAME being "cover"
func KequeenChapters(sup soup.Root) [][]string {
	var chs [][]string

	for _, img := range sup.Find("main", "id", "main").FindAll("img") {
		if href := img.Attrs()["src"]; strings.Contains(href, "cover") ||
			strings.Contains(href, "Cover") {
			chs = append(chs, []string{"cover", href})
			break
		}
	}

	for _, a := range sup.Find("main", "id", "main").FindAll("a") {
		chs = append(chs, []string{a.FullText(), a.Attrs()["href"]})
	}

	return chs
}

// Return chapter content, and extra files for chapter URL URL.
// Chapter name is given by CHAPTERNAME, and chapter no. by N.
func KequeenChapter(url, chaptername string, n int) ([]byte, []EpubFile) {
	h, err := Request(url)
	if err != nil {
		panic(err)
	}

	sup := soup.HTMLParse(h)
	div := sup.Find("div", "class", "entry-content").FindAll("div", "class", "elementor-widget-container")

	var content bytes.Buffer
	var extra []EpubFile
	// var imgBuf strings.Builder

	content.WriteString(EpubContentPreamble(chaptername))
	content.WriteString("<h1>")
	content.WriteString(chaptername)
	content.WriteString("</h1>\n")

	firsth2skipped := false
	imgCounter := 1

	for _, i := range div {
		if h2 := i.Find("h2", "class", "elementor-heading-title"); h2.Pointer != nil &&
			!firsth2skipped {
			firsth2skipped = true
			continue
		}
		if spacer := i.Find("div", "class", "elementor-spacer-inner"); spacer.Pointer != nil &&
			len(spacer.Children()) == 0 {
			continue
		}
		for _, c := range i.Children() {
			if c.Pointer.Data == "style" {
				continue
			}
			html := c.HTML()
			if c.Pointer.Data == "img" {
				html, imgCounter, extra =  ReplaceImgTags(
					html, []soup.Root{c},
					imgCounter, n, extra)
			}
			content.WriteString(html)
			content.WriteString("\n")
		}
	}
	content.WriteString(EpubContentEnd())

	return content.Bytes(), extra
}

// Return the page title for volume/series with soup SUP.
func KequeenSeriesTitle(sup soup.Root) string {
	title := sup.Find("head").Find("title")
	if title.Pointer == nil {
		return "***UNKNOWN***"
	}
	s := title.FullText()
	return strings.TrimSuffix(s, " – KequeenTLS")
}

// Return the files for the series with URL URL.
func KequeenEpubFiles(url string) map[string][]EpubFile {
	h, err := Request(url)
	if err != nil {
		panic(err)
	}
	sup := soup.HTMLParse(h)

	chapters := KequeenChapters(sup)
	seriesTitle := KequeenSeriesTitle(sup)

	var files []EpubFile
	n := 1
	for _, ch := range chapters {
		if ch[0] == "cover" {
			fmt.Println("Fetching volume cover")
			files = AddCoverImage(ch[1], files)
			continue
		}
		fmt.Println("Fetching", ch[1])
		content, extra := KequeenChapter(ch[1], ch[0], n+1)
		cid := "Chapter" + strconv.Itoa(n+1)
		files = append(files,
			EpubFile{
				Title: ch[0],
				Id: cid,
				Filename: "OEBPS/Text/" + cid + ".xhtml",
				Mimetype: "application/xhtml+xml",
				Content: content,
			})
		files = append(files, extra...)
		n++
	}
	return map[string][]EpubFile{
		seriesTitle: EpubAddExtra(
			"KequeenTLS",
			url, seriesTitle,
			files),
	}
}

// * NeoSekai Translations
var NeoSekaiAjaxUrl string = "https://www.neosekaitranslations.com/wp-admin/admin-ajax.php"

// Return the series title for the series soup SUP.
func NeoSekaiSeriesTitle(sup soup.Root) string {
	if div := sup.Find("div", "class", "post-title"); div.Pointer != nil {
		return strings.TrimSpace(div.Find("h1").Text())
	}

	return strings.TrimSuffix(
		strings.TrimSpace(sup.Find("title").Text()),
		"- NeoSekai Translations")

}

// Return a list of [ URL, CHAPTERNAME ] for the series soup SUP.
func NeoSekaiChapters(sup soup.Root) [][]string {
	id := sup.Find("div", "id", "manga-chapters-holder").Attrs()["data-id"]

	pdata := nurl.Values{}
	pdata.Set("action", "manga_get_chapters")
	pdata.Add("manga", id)

	req, err := PostForm(NeoSekaiAjaxUrl, pdata)
	if err != nil {
		return [][]string{}
	}

	sup = soup.HTMLParse(req)
	var ret [][]string
	for _, li := range sup.FindAll("li", "class", "wp-manga-chapter") {
		a := li.Find("a")
		ret = append([][]string{
			[]string{
				a.Attrs()["href"],
				strings.TrimSpace(a.Text()),
			}}, ret...)
	}

	return ret
}

// Return contents for chapter URL, title CHAPTERTITLE, and chapter no. N.
func NeoSekaiChapter(url, chapterTitle string, n int) ([]byte, []EpubFile) {
	var ret bytes.Buffer
	var extra []EpubFile

	h, err := Request(url)
	if err != nil {
		panic(err)
	}

	ret.WriteString(EpubContentPreamble(chapterTitle))

	s := soup.HTMLParse(h)
	div := s.Find("div", "class", "reading-content")
	imgCounter := 1
	for _, c := range div.Children() {
		if SoupTag(c) == "input" {
			continue
		} else if imgs := c.FindAll("img"); len(imgs) != 0 {
			var html string
			html, imgCounter, extra = ReplaceImgTags(
				c.HTML(), imgs, imgCounter, n, extra)
			ret.WriteString(html)
		} else {
			ret.WriteString(c.HTML())
		}
	}
	ret.WriteString(EpubContentEnd())

	return ret.Bytes(), extra
}

// Return the cover image url for the series soup SUP.
func NeoSekaiCoverUrl(sup soup.Root) string {
	if div := sup.Find("div", "class", "summary_image"); SoupTag(div) != "" {
		return div.Find("img").Attrs()["data-src"]
	}
	return ""
}

// Return the files for the series url URL.
func NeoSekaiEpubFiles(url string) map[string][]EpubFile {
	h, err := Request(url)
	if err != nil {
		panic(err)
	}
	sup := soup.HTMLParse(h)

	chapters := NeoSekaiChapters(sup)
	seriesTitle := NeoSekaiSeriesTitle(sup)
	var files []EpubFile

	cover := NeoSekaiCoverUrl(sup)
	if cover != "" {
		fmt.Println("Fetching cover image")
		files = AddCoverImage(cover, files)
	}

	n := 1
	for _, ch := range chapters {
		fmt.Println("Fetching", ch[1])
		content, extra := NeoSekaiChapter(ch[0], ch[1], n)
		cid := "Chapter" + strconv.Itoa(n+1)
		files = append(files,
			EpubFile{
				Title: ch[1],
				Id: cid,
				Filename: "OEBPS/Text/" + cid + ".xhtml",
				Mimetype: "application/xhtml+xml",
				Content: content,
			})
		files = append(files, extra...)
		n++
	}
	return map[string][]EpubFile{
		seriesTitle: EpubAddExtra(
			"NeoSekai Translations",
			url, seriesTitle, files),
	}
}

// * American Faux
// Return the series title for the series soup SUP.
func AmericanFauxSeriesTitle(sup soup.Root) string {
	return strings.TrimSpace(
		sup.Find("h1", "class", "entry-title").Text())
}

// Return chapter list [ TITLE, URL ] for the series soup SUP.
func AmericanFauxChapters(sup soup.Root) [][]string {
	var ret [][]string

	for _, a := range sup.FindAll("a", "data-type", "post") {
		ret = append(ret, []string{a.Attrs()["href"], a.Text()})
	}

	return ret
}

// Return content for chapter URL with TITLE and chapter no. N.
func AmericanFauxChapter(url, title string, n int) ([]byte, []EpubFile) {
	var ret bytes.Buffer
	var extra []EpubFile

	h, err := Request(url)
	if err != nil {
		panic(err)
	}

	ret.WriteString(EpubContentPreamble(title))

	s := soup.HTMLParse(h)
	div := s.Find("div", "class", "entry-content")
	imgCounter := 1
	for _, c := range div.Children() {
		if imgs := c.FindAll("img"); len(imgs) != 0 {
			var html string
			html, imgCounter, extra = ReplaceImgTags(
				c.HTML(), imgs, imgCounter, n, extra)
			ret.WriteString(html)
		} else if SoupTag(c) == "div" &&
			strings.HasPrefix(c.Attrs()["id"], "waldo-tag") {
			continue
		} else if SoupTag(c) == "hr" &&
			HtmlValueContains("wp-block-separator",
				c.Attrs()["class"]) {
			break
		} else {
			ret.WriteString(c.HTML())
		}
	}
	ret.WriteString(EpubContentEnd())

	return ret.Bytes(), extra
}

// Return the files for the series url URL.
func AmericanFauxEpubFiles(url string) map[string][]EpubFile {
	h, err := Request(url)
	if err != nil {
		panic(err)
	}
	sup := soup.HTMLParse(h)

	chapters := AmericanFauxChapters(sup)
	seriesTitle := AmericanFauxSeriesTitle(sup)
	var files []EpubFile

	n := 1
	for _, ch := range chapters {
		fmt.Println("Fetching", ch[1])
		content, extra := AmericanFauxChapter(ch[0], ch[1], n)
		cid := "Chapter" + strconv.Itoa(n+1)
		files = append(files,
			EpubFile{
				Title: ch[1],
				Id: cid,
				Filename: "OEBPS/Text/" + cid + ".xhtml",
				Mimetype: "application/xhtml+xml",
				Content: content,
			})
		files = append(files, extra...)
		n++
	}
	return map[string][]EpubFile{
		seriesTitle: EpubAddExtra(
			"American Faux",
			url, seriesTitle, files),
	}
}

// * My fiancé is in love with my little sister
// Site: http://hermitranslation.blogspot.com/p/index.html
var FianceChapterRe = regexp.MustCompile(`chapter-?[0-9]+(_[0-9]+)?\.html$`)

func FianceChapters(url string) [][]string {
	h, err := Request(url)
	if err != nil {
		panic(err)
	}
	sup := soup.HTMLParse(h)

	var ret [][]string
	n := 1
	for _, a := range sup.Find("div", "class", "post").FindAll("a") {
		u := a.Attrs()["href"]
		if FianceChapterRe.MatchString(u) {
			ret = append(ret,
				[]string{u, "Chapter " + strconv.Itoa(n)})
			n++
		}
	}

	return ret
}

// Return contents for chapter url URL with TITLE and chapter no. N.
func FianceChapter(url, title string, n int) ([]byte, []EpubFile) {
	var ret bytes.Buffer
	var extra []EpubFile

	h, err := Request(url)
	if err != nil {
		panic(err)
	}

	ret.WriteString(EpubContentPreamble(title))

	s := soup.HTMLParse(h)
	div := s.Find("div", "class", "post-body")
	imgCounter := 1
	for _, c := range div.Children() {
		if imgs := c.FindAll("img"); len(imgs) != 0 {
			var html string
			html, imgCounter, extra = ReplaceImgTags(
				c.HTML(), imgs, imgCounter, n, extra)
			ret.WriteString(html)
		} else {
			ret.WriteString(c.HTML())
		}
	}
	ret.WriteString(EpubContentEnd())

	return ret.Bytes(), extra
}

// Return the files for the series url URL.
func FianceEpubFiles(url string) map[string][]EpubFile {
	chapters := FianceChapters(url)
	seriesTitle := "My fiancé is in love with my little sister"
	var files []EpubFile

	n := 1
	for _, ch := range chapters {
		fmt.Println("Fetching", ch[1])
		content, extra := FianceChapter(ch[0], ch[1], n)
		cid := "Chapter" + strconv.Itoa(n+1)
		files = append(files,
			EpubFile{
				Title: ch[1],
				Id: cid,
				Filename: "OEBPS/Text/" + cid + ".xhtml",
				Mimetype: "application/xhtml+xml",
				Content: content,
			})
		files = append(files, extra...)
		n++
	}
	return map[string][]EpubFile{
		seriesTitle: EpubAddExtra(
			"Nocta's Hermit Den",
			url, seriesTitle, files),
	}
}

// * Apprentice Translations
// Return the series title for the soup SUP.
func ApprenticeSeriesTitle(sup soup.Root) string {
	return strings.TrimSpace(
		sup.Find("h1", "class", "entry-title").Text())
}

var ApprenticeOldUrlRe = regexp.MustCompile(`https://apprenticetranslations.com/`)
// Return [ URL, TITLE ] for the series soup SUP.
func ApprenticeChapters(sup soup.Root) [][]string {
	div := sup.Find("div", "class", "entry-content")

	var ret [][]string
	for _, a := range div.FindAll("a") {
		u := a.Attrs()["href"]
		if strings.Contains(u, "/?share=") || u == "" {
			continue
		}
		u = ApprenticeOldUrlRe.ReplaceAllString(u, "https://apprenticetranslations.wordpress.com/")
		ret = append(ret, []string{u, strings.TrimSpace(a.Text())})
	}

	return ret
}

var ApprenticeChButtonRe = regexp.MustCompile(`(Previous Chapter)?.*(Next Chapter)?`)

// Return contents for chapter url URL with TITLE and chapter no. N.
func ApprenticeChapter(url, title string, n int) ([]byte, []EpubFile) {
	var ret bytes.Buffer
	var extra []EpubFile

	h, err := Request(url)
	if err != nil {
		panic(err)
	}

	ret.WriteString(EpubContentPreamble(title))

	s := soup.HTMLParse(h)
	div := s.Find("div", "class", "entry-content")
	imgCounter := 1
	for _, c := range div.Children() {
		if imgs := c.FindAll("img"); len(imgs) != 0 {
			var html string
			html, imgCounter, extra = ReplaceImgTags(
				c.HTML(), imgs, imgCounter, n, extra)
			ret.WriteString(html)
		} else if a := c.FindAll("a"); len(a) != 0 {
			break
		} else {
			ret.WriteString(c.HTML())
		}
	}
	ret.WriteString(EpubContentEnd())

	return ret.Bytes(), extra
}

// Return the files for the series url URL.
func ApprenticeEpubFiles(url string) map[string][]EpubFile {
	h, err := Request(url)
	if err != nil {
		panic(err)
	}
	sup := soup.HTMLParse(h)

	chapters := ApprenticeChapters(sup)
	seriesTitle := ApprenticeSeriesTitle(sup)
	var files []EpubFile

	n := 1
	for _, ch := range chapters {
		fmt.Println("Fetching", ch[1])
		content, extra := ApprenticeChapter(ch[0], ch[1], n)
		cid := "Chapter" + strconv.Itoa(n)
		files = append(files,
			EpubFile{
				Title: ch[1],
				Id: cid,
				Filename: "OEBPS/Text/" + cid + ".xhtml",
				Mimetype: "application/xhtml+xml",
				Content: content,
			})
		files = append(files, extra...)
		n++
	}
	return map[string][]EpubFile{
		seriesTitle: EpubAddExtra(
			"Apprentice Translations",
			url, seriesTitle, files),
	}
}

// * Violet Evergarden
// Index: https://dennou-translations.tumblr.com/post/159331691639/violet-evergarden-novel-index

// Return a map of volume with key being [ URL, TITLE ] for the chapter.
// URL is the index url.
func VioletEvergardenVolumes(url string) map[string][][]string {
	h, err := Request(url)
	if err != nil {
		panic(err)
	}
	sup := soup.HTMLParse(h)

	ret := make(map[string][][]string)
	for _, h2 := range sup.Find("article", "class", "post").FindAll("h2") {
		title := strings.TrimSpace(h2.Text())
		var chps [][]string
		for _, a := range h2.FindNextSibling().FindAll("a") {
			chps = append(chps, []string{
				a.Attrs()["href"],
				strings.TrimSpace(a.Text())})
		}
		ret[title] = chps
	}

	return ret
}

// Return content for chapter URL with TITLE and chapter no. N.
func VioletEvergardenChapter(url, title string, n int) ([]byte, []EpubFile) {
	h, err := Request(url)
	if err != nil {
		panic(err)
	}
	sup := soup.HTMLParse(h)

	var ret bytes.Buffer
	var extra []EpubFile

	ret.WriteString(EpubContentPreamble(title))

	var div soup.Root
	if strings.Contains(url, "x0401x.tumblr.com") {
		div = sup.Find("div", "class", "container")
		if div.Pointer == nil {
			div = sup.Find("div", "class", "posts")
		}
	} else {
		div = sup.Find("article", "class", "post")
	}

	imgCounter := 1
	for _, c := range div.Children() {
		if imgs := c.FindAll("img"); len(imgs) != 0 {
			var html string
			html, imgCounter, extra = ReplaceImgTags(
				c.HTML(), imgs, imgCounter, n, extra)
			ret.WriteString(html)
		} else if SoupTag(c) == "div" && c.Attrs()["class"] == "tagged_post" {
			break
		} else {
			ret.WriteString(c.HTML())
		}
	}
	ret.WriteString(EpubContentEnd())

	return ret.Bytes(), extra
}

// Return the files for the index url URL.
func VioletEvergardenEpubFiles(url string) map[string][]EpubFile {
	ret := make(map[string][]EpubFile)
	vols := VioletEvergardenVolumes(url)
	for v, c := range vols {
		var files []EpubFile
		for n, ch := range c {
			fmt.Println("Fetching", ch[0])
			content, extra := VioletEvergardenChapter(ch[0], ch[1], n+1)
			cid := "Chapter" + strconv.Itoa(n+1)
			files = append(files,
				EpubFile{
					Title: ch[1],
					Id: cid,
					Filename: "OEBPS/Text/" + cid + ".xhtml",
					Mimetype: "application/xhtml+xml",
					Content: content,
				})
			files = append(files, extra...)
		}
		ret[v] = EpubAddExtra("Dennou Translations",
			url, "Violet Evergarden - " + v, files)
	}
	return ret
}

// * CClaw Translations
var CClawTitleRe = regexp.MustCompile(` ToC - CClaw Translations`)

// Return the title for the series soup SUP.
func CClawSeriesTitle(sup soup.Root) string {
	if h1 := sup.Find("h1", "class", "entry-title"); h1.Pointer != nil {
		return strings.TrimSuffix(strings.TrimSpace(h1.Text()), " ToC")
	} else {
		return CClawTitleRe.ReplaceAllString(
			strings.TrimSpace(sup.Find("title").Text()),
			"")
	}
}

// Return the cover URL for <img> with attrs A.
func CClawcoverurl(a map[string]string) string {
	if u, ok := a["data-large-file"]; ok {
		return u
	} else {
		return a["src"]
	}
}

// Return [ URL, TITLE ] for single volume TOC soup SUP.
func CClawsingleVol(sup soup.Root) [][]string {
	var ret [][]string
	div := sup.Find("div", "class", "entry-content")

	if img := div.Find("img"); img.Pointer != nil {
		ret = append(ret, []string{CClawcoverurl(img.Attrs()), "cover"})
	}

	for _, a := range div.FindAll("a") {
		if t := strings.TrimSpace(a.Text()); t != "" {
			ret = append(ret, []string{a.Attrs()["href"], t})
		}
	}

	return ret
}

// Return map of Volume -> [ URL, TITLE ] for series TOC soup SUP.
func CClawVolumes(sup soup.Root) map[string][][]string {
	ret := make(map[string][][]string)

	div := sup.Find("div", "class", "entry-content")
	if div.Find("h2").Pointer == nil {
		ret[CClawSeriesTitle(sup)] = CClawsingleVol(sup)
		return ret
	}

	imgs := div.FindAll("img")
	for i, h2 := range div.FindAll("h2") {
		chs := [][]string{[]string{CClawcoverurl(imgs[i].Attrs()), "cover"}}
		var vol string
		vol = strings.TrimSuffix(strings.TrimSpace(h2.Text()), " (Final)")
		for c := h2.FindNextSibling(); c.Pointer != nil && SoupTag(c) != "h2"; c = c.FindNextSibling() {
			if a := c.Find("a"); a.Pointer != nil && a.Text() != "" {
				chs = append(chs, []string{a.Attrs()["href"], a.Text()})
			}
		}
		ret[vol] = chs
	}
	return ret
}

// Return content for chapter URL with TITLE and chapter no. N.
func CClawChapter(url, title string, n int) ([]byte, []EpubFile) {
	h, err := Request(url)
	if err != nil {
		panic(err)
	}
	sup := soup.HTMLParse(h)

	var ret bytes.Buffer
	var extra []EpubFile

	ret.WriteString(EpubContentPreamble(title))
	imgCounter := 1
	do := func(c soup.Root) bool {
		if imgs := c.FindAll("img"); len(imgs) != 0 {
			var html string
			html, imgCounter, extra = ReplaceImgTags(
				c.HTML(), imgs, imgCounter, n, extra)
			ret.WriteString(html)
		} else if SoupTag(c) == "span" && HtmlValueContains("wordads-inline-marker", c.Attrs()["id"]) {
			return false
		} else if SoupTag(c) == "div" && strings.HasPrefix(c.Attrs()["id"], "atatags-") {
			return false
		} else {
			ret.WriteString(c.HTML())
		}
		return false
	}
	if c := sup.Find("h2", "class", "wp-block-heading"); c.Pointer != nil {
		for c = c; c.Pointer != nil; c = c.FindNextSibling() {
			if !do(c) {
				break
			}
		}
	} else {
		for _, c := range sup.Find("div", "class", "entry-content").Children() {
			if !do(c) {
				break
			}
		}
	}
	ret.WriteString(EpubContentEnd())

	return ret.Bytes(), extra
}

// Return the files for the series TOC page URL URL.
func CClawEpubFiles(url string) map[string][]EpubFile {
	h, err := Request(url)
	if err != nil {
		panic(err)
	}
	sup := soup.HTMLParse(h)
	seriesTitle := CClawSeriesTitle(sup)

	ret := make(map[string][]EpubFile)
	vols := CClawVolumes(sup)
	for v, c := range vols {
		var files []EpubFile
		n := 1
		for _, ch := range c {
			fmt.Println("Fetching", v, ch[1], ch[0])
			if ch[1] == "cover" {
				files = AddCoverImage(ch[0], files)
				continue
			}
			content, extra := CClawChapter(ch[0], ch[1], n)
			cid := "Chapter" + strconv.Itoa(n)
			files = append(files,
				EpubFile{
					Title: ch[1],
					Id: cid,
					Filename: "OEBPS/Text/" + cid + ".xhtml",
					Mimetype: "application/xhtml+xml",
					Content: content,
				})
			files = append(files, extra...)
			n++
		}
		ret[seriesTitle + " - " + v] = EpubAddExtra("CClaw Translations", c[0][0], seriesTitle + " - " + v, files)
	}
	return ret
}

func main() {
	if len(os.Args) == 1 {
		fmt.Println(`usage: ln2epub URL...`)
		os.Exit(1)
	}

	files := make(map[string][]EpubFile)
	for _, u := range os.Args[1:] {
		switch {
		case strings.Contains(u, "soafp.com"):
			files = SoafpEpubFiles(u)
		case strings.Contains(u, "shalvationtranslations.wordpress.com"):
			files = ShalvationEpubFiles(u)
		case strings.Contains(u, "baka-tsuki.org"):
			files = BakatsukiEpubFiles(u)
		case strings.Contains(u, "travistranslations.com/novel/"):
			files = TravisEpubFiles(u)
		case strings.Contains(u, "kequeentls.com"):
			files = KequeenEpubFiles(u)
		case strings.Contains(u, "neosekaitranslations.com"):
			files = NeoSekaiEpubFiles(u)
		case strings.Contains(u, "americanfaux.com"):
			files = AmericanFauxEpubFiles(u)
		case strings.Contains(u, "hermitranslation.blogspot.com"):
			files = FianceEpubFiles(u)
		case strings.Contains(u, "apprenticetranslations.wordpress.com"):
			files = ApprenticeEpubFiles(u)
		case strings.Contains(u, "violet-evergarden-novel-index"):
			files = VioletEvergardenEpubFiles(u)
		case strings.Contains(u, "cclawtranslations.home.blog/"):
			files = CClawEpubFiles(u)
		}

		for uu, ef := range files {
			f := EpubFileName(uu)
			EpubCreateFile(f, ef)
			fmt.Println("Created epub file", f, "for", uu)
		}
	}
}

// Local Variables:
// compile-command: "go run ln2epub.go"
// outline-regexp: "// \\(\\*+\\) \\|^func \\|^type "
// eval: (outline-minor-mode)
// eval: (reveal-mode)
// End:
