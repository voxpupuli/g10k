%{
package g10k
%}

%union {
	str   string
	attr  pfAttribute
	attrs []pfAttribute
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
		lex.state.handleModuledir($2)
	}
| FORGEBASEURL value NEWLINE
	{
		lex := pfyylex.(*pfLexer)
		lex.state.handleForgeBaseURL($2)
	}
| FORGECACHETTL value NEWLINE
	{
		lex := pfyylex.(*pfLexer)
		lex.state.err = lex.state.handleForgeCacheTTL($2, lex.currentLine)
		if lex.state.err != nil {
			return 1
		}
	}
| MOD STRING opt_attrs NEWLINE
	{
		lex := pfyylex.(*pfLexer)
		lex.state.err = lex.state.handleMod($2, $3, lex.currentLine)
		if lex.state.err != nil {
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
		$$ = []pfAttribute{$1}
	}
| attr_list COMMA attr
	{
		$$ = append($1, $3)
	}
;

attr:
	SYMBOL ARROW value
	{
		$$ = pfAttribute{Name: $1, Value: $3, HasArrow: true}
	}
| SYMBOL
	{
		$$ = pfAttribute{Name: $1, Value: "", HasArrow: false}
	}
| STRING
	{
		$$ = pfAttribute{Value: $1, IsBareValue: true}
	}
| WORD
	{
		$$ = pfAttribute{Value: $1, IsBareValue: true}
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
