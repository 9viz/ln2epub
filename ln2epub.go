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
		if i.Mimetype != "application/xhtml+xml" {
			continue
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
// This returns the image file contents, image mimetype.
func FetchImage(url string) ([]byte, string) {
	img, _ := fetch(url)
	return img, http.DetectContentType(img)
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

		if c.Pointer.Data == "div" && SoafpisEnd(c.Attrs()["class"]) {
			break
		}

		if c.Pointer.Data == "div" && SoafpisImg(c.Attrs()["class"]) {
			imgCounter += 1
			imgAttrs := c.Find("img").Attrs()
			img, mimetype := FetchImage(imgAttrs["src"])
			imgId := fmt.Sprintf("Ch%d_img%d", n, imgCounter)
			imgFileName := "Images/" + imgId

			ret.WriteString("<img src='../")
			ret.WriteString(imgFileName)
			ret.WriteString("' ")
			for _, a := range []string{"width", "height", "alt"} {
				if w, ok := imgAttrs[a]; ok {
					ret.WriteString(fmt.Sprintf("%s='%s' ", a, w))
				}
			}
			ret.WriteString("/>\n")

			extra = append(extra,
				EpubFile{
					Id:       imgId,
					Content:  img,
					Filename: "OEBPS/" + imgFileName,
					Mimetype: mimetype,
				})

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
func SoafpEpubFiles(url string) []EpubFile {
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
	return EpubAddExtra("", title, title, files)
}

// * Common routines

// Return an approriate epub filename for URL.
// TODO: Should include an optional part to indicate how many chapters
// have been fetched.
func EpubFileName(url string) string {
	return path.Base(strings.TrimSuffix(url, "/")) + ".epub"
}

func main() {
	if len(os.Args) == 1 {
		fmt.Println(`usage: ln2epub URL...`)
		os.Exit(1)
	}

	for _, u := range os.Args[1:] {
		f := EpubFileName(u)
		switch {
		case strings.Contains(u, "soafp.com"):
			EpubCreateFile(f, SoafpEpubFiles(u))
			fmt.Println("Created epub file", f, "for", u)
		}
	}
}

// Local Variables:
// outline-regexp: "// \\(\\*+\\) "
// eval: (outline-minor-mode)
// eval: (reveal-mode)
// End:
