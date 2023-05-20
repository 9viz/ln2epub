
// Find TAG in HTML TOKENIZER that whose attributes satisfy ATTRCOND.
// ATTRCOND is called is called with two arguments: KEY and VALUE.  If
// ATTRCOND is nil, then ignore attributes and select the tag
// directly.  Errors encountered while tokenising are simply ignored!
func HtmlFind(tokenizer *html.Tokenizer, tag string, attrCond func(string, string) bool) string {
	depth := 0
	tagDepth := -1
	var ret strings.Builder
	loop := true
	for loop {
		ttype := tokenizer.Next()
		token := tokenizer.Token()
		switch ttype {
		case html.ErrorToken:
			if tokenizer.Err() == io.EOF {
				loop = false
			}
		case html.StartTagToken:
			depth += 1
			if token.Data == tag {
				if attrCond == nil {
					tagDepth = depth
					continue
				}
			AttrLoop:
				for _, i := range token.Attr {
					if attrCond(i.Key, i.Val) {
						tagDepth = depth
						break AttrLoop
					}
				}
			}
		case html.SelfClosingTagToken:
			if token.Data == tag {
				continue
				if attrCond == nil {
					// We just need it to be not -1.
					tagDepth = 1
					loop = false
					continue
				}
				for _, i := range token.Attr {
					if attrCond(i.Key, i.Val) {
						loop = false
						tagDepth = 1
					}
				}
			}
		case html.EndTagToken:
			if token.Data == tag && depth == tagDepth {
				loop = false
			}
			depth -= 1
		}
		if tagDepth != -1 {
			ret.WriteString(token.String())
		}
	}

	return ret.String()
}

// Return a function to use in HtmlFind* that returns true if KEY
// attribute contains VAL as a value.
func HtmlKeyInAttr(key, value string) func(string, string) bool {
	return func(k, v string) bool {
		if k == key {
			if strings.Contains(v, " ") {
				vs := strings.Split(v, " ")
				for _, i := range vs {
					if i == value {
						return true
					}
				}
			} else {
				return v == value
			}
		}
		return false
	}
}

// Return first TAG whose attr KEY is equal to VALUE.
func HtmlFindByAttr(tokenizer *html.Tokenizer, tag, key, value string) string {
	return HtmlFind(tokenizer, tag, HtmlKeyInAttr(key, value))
}

// Return true if F returns true for ATTR attributes.
// If F is nil, then always return true
func HtmlCheckAttrs(f func(string, string) bool, attr []html.Attribute) bool {
	if f == nil {
		return true
	}
	for _, i := range attr {
		if f(i.Key, i.Val) {
			return true
		}
	}
	return false
}

func HtmlFindAll(tokenizer *html.Tokenizer, tag string, attrCond func(string, string) bool) []string {
	depth := 0
	var tagDepth []int
	var h []*strings.Builder
	var ret []string

	push := false // Whether to push H to RET.
	loop := true
	n := -1
	for loop {
		ttype := tokenizer.Next()
		token := tokenizer.Token()

		switch {
		case ttype == html.ErrorToken && tokenizer.Err() == io.EOF:
			loop = false
		case ttype == html.SelfClosingTagToken && token.Data == tag && HtmlCheckAttrs(attrCond, token.Attr):
			// Add a dummy elemnt.
			n += 1
			tagDepth = append(tagDepth, 1)
			h = append(h, &strings.Builder{})
			push = true
		case ttype == html.StartTagToken:
			depth += 1
			if token.Data == tag && HtmlCheckAttrs(attrCond, token.Attr) {
				n += 1
				tagDepth = append(tagDepth, depth)
				h = append(h, &strings.Builder{})
			}
		case ttype == html.EndTagToken:
			if token.Data == tag && depth == tagDepth[n] {
				push = true
			}
			depth -= 1
		}

		str := token.String()
		for _, i := range h {
			i.WriteString(str)
		}

		if push {
			ret = append(ret, h[n].String())
			tagDepth = tagDepth[:n]
			h = h[:n]
			n -= 1
			push = false
		}
	}

	return ret
}


// Return the content for chapter URL and CHAPTERNAME.
func SoapfGetChapter(url, chaptername string) []byte {
	var ret bytes.Buffer

	h, err := Request(url)
	if err != nil {
		panic(err)
	}

	tokenizer := html.NewTokenizer(strings.NewReader(h))
	h = HtmlFindByAttr(tokenizer, "div", "class", "entry-content")
	tokenizer = html.NewTokenizer(strings.NewReader(h))

	loop := true
	skip := false // Whether to skip this tag
	inAd := false
	depth := 0
	adDepth := -1
	for loop {
		ttype := tokenizer.Next()
		token := tokenizer.Token()

		switch {
		case ttype == html.ErrorToken && tokenizer.Err() == io.EOF:
			loop = false
		case ttype == html.StartTagToken:
			depth += 1
			fallthrough
		case ttype == html.StartTagToken &&
			((token.Data == "div" &&
				HtmlCheckAttrs(HtmlKeyInAttr("class", "code-block"), token.Attr)) ||
			(token.Data == "span" &&
				HtmlCheckAttrs(HtmlKeyInAttr("class", "has-inline-color"), token.Attr))):
			skip = true
			adDepth = depth
			inAd = true
		case ttype == html.EndTagToken &&
			(token.Data == "div" || token.Data == "span") &&
			depth == adDepth:
			inAd = false
			adDepth = -1
			fallthrough
		case ttype == html.EndTagToken:
			depth -= 1
		case ttype == html.SelfClosingTagToken && token.Data == "hr" &&
			HtmlCheckAttrs(HtmlKeyInAttr("class", "wp-block-separator"), token.Attr):
			loop = false
			skip = true
		}
		if !skip {
			ret.WriteString(token.String())
		}
		if skip && !inAd {
			skip = false
		}
	}

	// Add the final </div>.
	ret.WriteString("</div>")
	return ret.Bytes()
}
