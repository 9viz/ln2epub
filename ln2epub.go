// TODO: Should generalise <img> rewriting code in all chapter fetch
// functions.
package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"github.com/anaskhan96/soup"
	// "golang.org/x/net/proxy"
	"io/ioutil"
	"net/http"
	// nurl "net/url"
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
	var cover EpubFile

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
	content.WriteString("</metadata>\n\n")

	// Manifest section.
	content.WriteString("<manifest>")
	for _, i := range files {
		if i.Id == "cover" {
			cover = i
		}
		content.WriteString("\n<item id='")
		content.WriteString(i.Id)
		content.WriteString("' href='")
		content.WriteString(EpubstripOebpsPrefix(i.Filename))
		content.WriteString("' media-type='")
		content.WriteString(i.Mimetype)
		content.WriteString("' />")
	}
	content.WriteString("\n<item id='ncx' href='toc.ncx' media-type='application/x-dtbncx+xml'/>\n</manifest>\n")

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

	// TODO: Should also include <meta name="cover"> in the
	// metadata section.
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

// Make a GET request for URL.
func fetch(url string) ([]byte, error) {
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header = RHEADERS
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
	b, e := fetch(url)
	return string(b), e
}

// Fetch the image from url URL.
// Return the image file contents, image mimetype.
func FetchImage(url string) ([]byte, string) {
	img, _ := fetch(url)
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

// * Soafp

// TODO: Get description of the series and cover image.

var SoapcleanChNameRe = regexp.MustCompile(`([[:space:]]+:[[:space:]]+)`)

// Return the series title with URL URL.
func SoafpGetSeriesTitle(url string) string {
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
func SoafpGetChapters(url string) [][]string {
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
func SoafpGetChapter(url, chaptername string, n int) ([]byte, []EpubFile) {
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
	chs := SoafpGetChapters(url)

	for n, ch := range chs {
		fmt.Println("Fetching chapter", ch[0])
		content, extra := SoafpGetChapter(ch[0], ch[1], n)
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

	title := SoafpGetSeriesTitle(url)
	return map[string][]EpubFile{
		url: EpubAddExtra("", title, title, files),
	}
}

// * Shalvation Translations

// Get the author of the series with TOC page soup SP.
func ShalvationGetAuthor(sp soup.Root) string {
	// The first <string> tag is for the author.
	strong := sp.Find("div", "class", "entry-content").Find("strong")
	// Needs to be HTML().  Text() and FullText() both return an
	// empty string.
	return strong.FindNextSibling().HTML()
}

// Return the cover image URL of the series with TOC page soup SP.
func ShalvationGetCoverImg(sp soup.Root) string {
	return sp.Find("div", "class", "entry-content").Find("img").Attrs()["src"]
}

// Return the synposis HTML for the series with TOC page soup SP.
// An empty string is returned if there is none.
func ShalvationGetSynopsis(sp soup.Root) string {
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
func ShalvationGetVols(sp soup.Root) map[string][][]string {
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
func ShalvationGetChapter(url, chaptername string, n int) ([]byte, []EpubFile) {
	var content bytes.Buffer
	var imgBuf strings.Builder
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
			for _, img := range imgs {
				ifile, ic := FetchImageCached(img.Attrs()["src"], n, imgCounter)
				// New image.
				if ic != imgCounter {
					extra = append(extra, ifile)
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

				if c.Text() == "" {
					content.WriteString("<p>")
					content.WriteString(imgBuf.String())
					content.WriteString("</p>")
				} else {
					content.WriteString(
						strings.ReplaceAll(c.HTML(),
							img.HTML(),
							imgBuf.String()))
				}
				content.WriteString("\n")
				imgBuf.Reset()
			}
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
			cover, mimetype := FetchImage(l[1])
			c := EpubFile{
				Id: "_cover-image",
				Filename: "OEBPS/Images/cover",
				Mimetype: mimetype,
				Content: cover}
			files = append(files, c)
			ImageCache[l[1]] = c

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
		case "Illustrations":
			fmt.Println("Fetching illustrations...")
			f, e := ShalvationGetChapter(l[1], "Illustrations", -1)
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
			f, e := ShalvationGetChapter(l[1], l[0], n)
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
func ShalvationGetTitle(sp soup.Root) string {
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
	vols := ShalvationGetVols(sp)
	author := ShalvationGetAuthor(sp)
	title := ShalvationGetTitle(sp)

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
func BakatsukiGetVolumes(url string) [][]string {
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
		var imgBuf strings.Builder

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
			if imgs := s.FindAll("img"); len(imgs) != 0 {
				for _, im := range imgs {
					ifile, ic := FetchImageCached(
						"https://baka-tsuki.org" + im.Attrs()["src"],
						chi+1, imgCounter)
					if ic != imgCounter {
						files = append(files, ifile)
						imgCounter = ic
					}
					imgBuf.WriteString("<img src='../")
					imgBuf.WriteString(EpubstripOebpsPrefix(ifile.Filename))
					imgBuf.WriteString("' ")
					for _, a := range []string{"width", "alt", "height"} {
						if at, ok := im.Attrs()[a]; ok {
							imgBuf.WriteString(a)
							imgBuf.WriteString("='")
							imgBuf.WriteString(at)
							imgBuf.WriteString("' ")
						}
					}
					imgBuf.WriteString("/>")
					str = strings.ReplaceAll(
						str,
						im.HTML(),
						imgBuf.String(),
					)
					imgBuf.Reset()
				}
			}
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
	vols := BakatsukiGetVolumes(url)

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
func TravisGetChapters(sup soup.Root) [][]string {
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
func TravisGetChapter(url, chaptername string, n int) ([]byte, []EpubFile) {
	h, err := Request(url)
	if err != nil {
		panic(err)
	}

	var content bytes.Buffer
	var imgBuf strings.Builder
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
		html := p.HTML()
		if imgs :=  p.FindAll("img"); len(imgs) != 0 {
			for _, img := range imgs {
				ifile, ic := FetchImageCached(img.Attrs()["src"], n, imgCounter)
				// New image.
				if ic != imgCounter {
					extra = append(extra, ifile)
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
				imgBuf.Reset()
			}
			continue
		}

		content.WriteString(html)
	}

	return content.Bytes(), extra
}

// Return the series title for series soup SUP.
func TravisGetSeriesTitle(sup soup.Root) string {
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

	chapters := TravisGetChapters(sup)
	seriesTitle := TravisGetSeriesTitle(sup)

	var files []EpubFile
	for n, ch := range chapters {
		fmt.Println("Fetching", ch[1])
		content, extra := TravisGetChapter(ch[1], ch[0], n+1)
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
		url: EpubAddExtra(
			"Travis Translations",
			url, seriesTitle,
			files),
	}
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
// outline-regexp: "// \\(\\*+\\)\\|^func \\|^type "
// eval: (outline-minor-mode)
// eval: (reveal-mode)
// End:
