%{
package puppetfileparser
%}

%union {
	str   string
	attr  Attribute
	attrs []Attribute
}

%token MOD MODULEDIR FORGEBASEURL FORGECACHETTL
%token COMMA ARROW NEWLINE
%token <str> STRING WORD SYMBOL

%type <str> value
%type <attr> attr
%type <attrs> opt_attrs attr_list

%start file

%%

file:
	/* empty */
| file line
;

line:
	NEWLINE
| MODULEDIR value NEWLINE
	{
		lex := pfyylex.(*pfLexer)
		lex.err = lex.handler.HandleModuleDir($2)
		if lex.err != nil {
			return 1
		}
	}
| FORGEBASEURL value NEWLINE
	{
		lex := pfyylex.(*pfLexer)
		lex.err = lex.handler.HandleForgeBaseURL($2)
		if lex.err != nil {
			return 1
		}
	}
| FORGECACHETTL value NEWLINE
	{
		lex := pfyylex.(*pfLexer)
		lex.err = lex.handler.HandleForgeCacheTTL($2, lex.currentLine)
		if lex.err != nil {
			return 1
		}
	}
| MOD STRING opt_attrs NEWLINE
	{
		lex := pfyylex.(*pfLexer)
		lex.err = lex.handler.HandleModule($2, $3, lex.currentLine)
		if lex.err != nil {
			return 1
		}
	}
;

opt_attrs:
	/* empty */
	{
		$$ = nil
	}
| COMMA attr_list
	{
		$$ = $2
	}
;

attr_list:
	attr
	{
		$$ = []Attribute{$1}
	}
| attr_list COMMA attr
	{
		$$ = append($1, $3)
	}
;

attr:
	SYMBOL ARROW value
	{
		$$ = Attribute{Name: $1, Value: $3, HasArrow: true}
	}
| SYMBOL
	{
		$$ = Attribute{Name: $1, Value: "", HasArrow: false}
	}
| STRING
	{
		$$ = Attribute{Value: $1, IsBareValue: true}
	}
| WORD
	{
		$$ = Attribute{Value: $1, IsBareValue: true}
	}
| WORD value
	{
		lex := pfyylex.(*pfLexer)
		if len($1) < 2 || $1[len($1)-1] != ':' {
			lex.Error("syntax error")
			return 1
		}
		$$ = Attribute{Name: $1[:len($1)-1], Value: $2, IsKeyword: true}
	}
;

value:
	STRING
	{
		$$ = $1
	}
| WORD
	{
		$$ = $1
	}
| SYMBOL
	{
		$$ = ":" + $1
	}
;

%%
