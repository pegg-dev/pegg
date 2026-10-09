package components

import (
	"sync"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/gdamore/tcell/v2"
	"github.com/peggco/pegg/internal/tui/styles"
)

var lexerCache sync.Map

func getCachedLexer(lang string) chroma.Lexer {
	if v, ok := lexerCache.Load(lang); ok {
		return v.(chroma.Lexer)
	}
	var l chroma.Lexer
	if lang == "" {
		l = lexers.Fallback
	} else {
		l = lexers.Get(lang)
		if l == nil {
			l = lexers.Fallback
		}
	}
	l = chroma.Coalesce(l)
	lexerCache.Store(lang, l)
	return l
}

func tokenTypeToThemeColor(tt chroma.TokenType, th styles.Theme) tcell.Color {
	switch tt.Category() {
	case chroma.Keyword:
		switch tt {
		case chroma.KeywordType:
			return th.TokenType
		default:
			return th.TokenKeyword
		}
	case chroma.Name:
		switch {
		case tt == chroma.NameFunction || tt == chroma.NameClass:
			return th.TokenFunction
		case tt == chroma.NameBuiltin || tt == chroma.NameConstant:
			return th.TokenConstant
		case tt == chroma.NameVariable || tt == chroma.NameVariableAnonymous ||
			tt == chroma.NameVariableClass || tt == chroma.NameVariableGlobal ||
			tt == chroma.NameVariableInstance:
			return th.TokenName
		case tt == chroma.NameDecorator:
			return th.TokenFunction
		default:
			return th.TokenName
		}
	case chroma.Literal:
		switch tt {
		case chroma.LiteralString, chroma.LiteralStringAffix, chroma.LiteralStringChar,
			chroma.LiteralStringDouble, chroma.LiteralStringSingle, chroma.LiteralStringHeredoc,
			chroma.LiteralStringInterpol, chroma.LiteralStringRegex, chroma.LiteralStringSymbol,
			chroma.LiteralStringEscape, chroma.LiteralStringOther:
			return th.TokenString
		case chroma.LiteralNumber, chroma.LiteralNumberBin, chroma.LiteralNumberFloat,
			chroma.LiteralNumberHex, chroma.LiteralNumberInteger, chroma.LiteralNumberOct,
			chroma.LiteralNumberByte:
			return th.TokenNumber
		case chroma.LiteralDate:
			return th.TokenConstant
		}
		return th.Foreground
	case chroma.Comment:
		return th.TokenComment
	case chroma.Operator:
		return th.TokenOperator
	case chroma.Punctuation:
		return th.TokenPunctuation
	case chroma.Generic:
		return th.Foreground
	case chroma.Error:
		return th.Error
	}
	return th.Foreground
}

func highlightLine(text string, filePath string, th styles.Theme, contentStyle tcell.Style) Line {
	lang := langFromFilename(filePath)
	l := getCachedLexer(lang)
	return highlightWithLexer(l, text, th, contentStyle)
}

func highlightByLangName(text string, lang string, th styles.Theme, contentStyle tcell.Style) Line {
	l := getCachedLexer(lang)
	return highlightWithLexer(l, text, th, contentStyle)
}

func highlightWithLexer(l chroma.Lexer, text string, th styles.Theme, contentStyle tcell.Style) Line {
	iterator, err := l.Tokenise(nil, text)
	if err != nil {
		return lineFromPlain(text, contentStyle)
	}

	baseStyle := th.Base().Background(th.Background)
	var out Line
	for _, tok := range iterator.Tokens() {
		colour := tokenTypeToThemeColor(tok.Type, th)
		style := baseStyle.Foreground(colour)
		for _, r := range tok.Value {
			out = append(out, Cell{R: r, S: style})
		}
	}
	if len(out) == 0 {
		return lineFromPlain(text, contentStyle)
	}
	return out
}

func langFromFilename(filePath string) string {
	l := lexers.Match(filePath)
	if l != nil {
		return l.Config().Name
	}
	return ""
}

func lineFromPlain(text string, style tcell.Style) Line {
	var out Line
	for _, r := range text {
		out = append(out, Cell{R: r, S: style})
	}
	return out
}
