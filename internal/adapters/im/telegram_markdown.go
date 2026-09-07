package im

import "strings"

// markdownToTelegramHTML 把通知正文的 Markdown 子集转换为 Telegram HTML。
// 子集覆盖协调器通知正文的全部记号:##/### 标题、- 列表、> 引用、**加粗**、
// `行内代码`、[文本](http(s) 链接)。其余字符一律按 HTML 转义输出,正文里的
// 字面 <> 不会变成标签;非 http(s) scheme 的链接按原样文本输出。本函数是全
// 函数:任何无法识别的结构都退化为转义文本,保证转换层不会丢消息。
func markdownToTelegramHTML(body string) string {
	lines := strings.Split(body, "\n")
	rendered := make([]string, len(lines))
	for i, line := range lines {
		rendered[i] = renderMarkdownLine(line)
	}
	return strings.Join(rendered, "\n")
}

func renderMarkdownLine(line string) string {
	// Telegram 无标题层级,各级标题统一加粗;按最长前缀优先匹配。
	for _, prefix := range []string{"### ", "## ", "# "} {
		if heading, ok := strings.CutPrefix(line, prefix); ok {
			return "<b>" + renderMarkdownInline(heading) + "</b>"
		}
	}
	if quoted, ok := strings.CutPrefix(line, "> "); ok {
		return "<blockquote>" + renderMarkdownLine(quoted) + "</blockquote>"
	}
	if bare, ok := strings.CutPrefix(line, ">"); ok {
		return "<blockquote>" + renderMarkdownLine(bare) + "</blockquote>"
	}
	if item, ok := strings.CutPrefix(line, "- "); ok {
		return "• " + renderMarkdownInline(item)
	}
	return renderMarkdownInline(line)
}

func renderMarkdownInline(text string) string {
	var out strings.Builder
	for i := 0; i < len(text); {
		switch c := text[i]; {
		case c == '`':
			if end := strings.IndexByte(text[i+1:], '`'); end >= 0 {
				out.WriteString("<code>")
				out.WriteString(escapeTelegramHTML(text[i+1 : i+1+end]))
				out.WriteString("</code>")
				i += end + 2
				continue
			}
			out.WriteByte('`')
			i++
		case c == '*' && i+1 < len(text) && text[i+1] == '*':
			if end := strings.Index(text[i+2:], "**"); end >= 0 {
				out.WriteString("<b>")
				out.WriteString(renderMarkdownInline(text[i+2 : i+2+end]))
				out.WriteString("</b>")
				i += end + 4
				continue
			}
			out.WriteString("**")
			i += 2
		case c == '[':
			label, url, ok := matchMarkdownLink(text, i)
			if !ok {
				out.WriteString("[")
				i++
				continue
			}
			out.WriteString(`<a href="`)
			out.WriteString(escapeTelegramHref(url))
			out.WriteString(`">`)
			out.WriteString(renderMarkdownInline(label))
			out.WriteString("</a>")
			i += len(label) + len(url) + 4
		case c == '<' || c == '>' || c == '&':
			out.WriteString(escapeTelegramHTML(text[i : i+1]))
			i++
		default:
			out.WriteByte(c)
			i++
		}
	}
	return out.String()
}

// matchMarkdownLink 在 text[start:] 处尝试匹配 [label](url)。url 必须是
// http(s) 链接且不含空白与引号,其余 scheme 按普通文本回退,防止把
// javascript: 之类内容注入 href。
func matchMarkdownLink(text string, start int) (label, url string, ok bool) {
	rest := text[start+1:]
	closeIdx := strings.IndexByte(rest, ']')
	if closeIdx < 0 {
		return "", "", false
	}
	label = rest[:closeIdx]
	after := rest[closeIdx+1:]
	if !strings.HasPrefix(after, "(") {
		return "", "", false
	}
	urlRest := after[1:]
	endIdx := strings.IndexByte(urlRest, ')')
	if endIdx < 0 {
		return "", "", false
	}
	url = urlRest[:endIdx]
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return "", "", false
	}
	if strings.ContainsAny(url, " \"'<>") {
		return "", "", false
	}
	return label, url, true
}

func escapeTelegramHref(url string) string {
	return strings.NewReplacer("&", "&amp;", `"`, "&quot;").Replace(url)
}
