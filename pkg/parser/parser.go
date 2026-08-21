package parser

import (
	"strings"
	"pcl/pkg/services"
)

// PipelineNode represents a command pipeline connected by '|' or tapped with '|> $var'.
type PipelineNode struct {
	Commands []*CommandNode
	TapVars  []string // TapVars[i] is variable name to capture output of Commands[i]
}

// CommandNode represents a single command invocation with arguments and redirections.
type CommandNode struct {
	Tokens       []Token
	Redirections []services.Redirection
	IsAssignment bool
	AssignTarget string
	AssignValue  *CommandNode
	PromptOpt    string // options after 'with'
}

// ScriptNode represents a sequence of pipeline statements.
type ScriptNode struct {
	Statements []*PipelineNode
}

// Parser translates tokens into executable AST structures.
type Parser struct {
	tokens []Token
	pos    int
}

func NewParser(tokens []Token) *Parser {
	return &Parser{tokens: tokens, pos: 0}
}

func (p *Parser) peek() Token {
	if p.pos >= len(p.tokens) {
		return Token{Type: TokEOF}
	}
	return p.tokens[p.pos]
}

func (p *Parser) advance() Token {
	tok := p.peek()
	if p.pos < len(p.tokens) {
		p.pos++
	}
	return tok
}

// ParseScript parses full input script into ScriptNode.
func (p *Parser) ParseScript() (*ScriptNode, error) {
	script := &ScriptNode{
		Statements: make([]*PipelineNode, 0),
	}

	for p.peek().Type != TokEOF {
		// Skip empty lines/separators
		if p.peek().Type == TokEOL {
			p.advance()
			continue
		}

		pipeline, err := p.parsePipeline()
		if err != nil {
			return nil, err
		}

		if pipeline != nil && len(pipeline.Commands) > 0 {
			script.Statements = append(script.Statements, pipeline)
		}
	}

	return script, nil
}

func (p *Parser) parsePipeline() (*PipelineNode, error) {
	pipeline := &PipelineNode{
		Commands: make([]*CommandNode, 0),
		TapVars:  make([]string, 0),
	}

	for p.peek().Type != TokEOF && p.peek().Type != TokEOL {
		cmd, err := p.parseCommand()
		if err != nil {
			return nil, err
		}

		if cmd != nil {
			pipeline.Commands = append(pipeline.Commands, cmd)
		}

		if p.peek().Type == TokPipeTap {
			p.advance() // consume '|>'
			tapVar := ""
			if p.peek().Type != TokEOF && p.peek().Type != TokEOL && p.peek().Type != TokPipe {
				varTok := p.advance()
				tapVar = strings.TrimPrefix(varTok.Value, "$")
			}
			pipeline.TapVars = append(pipeline.TapVars, tapVar)

			if p.peek().Type == TokPipe {
				p.advance() // consume '|'
			}
			continue
		} else if p.peek().Type == TokPipe {
			p.advance() // consume '|'
			pipeline.TapVars = append(pipeline.TapVars, "")
			continue
		} else {
			pipeline.TapVars = append(pipeline.TapVars, "")
			break
		}
	}

	// Consume EOL if present
	if p.peek().Type == TokEOL {
		p.advance()
	}

	return pipeline, nil
}

func (p *Parser) parseCommand() (*CommandNode, error) {
	cmd := &CommandNode{
		Tokens:       make([]Token, 0),
		Redirections: make([]services.Redirection, 0),
	}

	for {
		tok := p.peek()
		if tok.Type == TokEOF || tok.Type == TokEOL || tok.Type == TokPipe || tok.Type == TokPipeTap {
			break
		}

		// Handle Assignment syntax sugar: varName = <value>
		if len(cmd.Tokens) == 1 && tok.Type == TokAssign {
			p.advance() // consume '='
			target := cmd.Tokens[0].Value
			// Trim leading $ if present in target
			target = strings.TrimPrefix(target, "$")

			// Parse RHS command/value
			rhsCmd, err := p.parseCommand()
			if err != nil {
				return nil, err
			}

			return &CommandNode{
				IsAssignment: true,
				AssignTarget: target,
				AssignValue:  rhsCmd,
			}, nil
		}

		// Handle Redirections
		if tok.Type == TokRedirectIn {
			p.advance()
			targetTok := p.advance()
			cmd.Redirections = append(cmd.Redirections, services.Redirection{
				Type:   services.RedirectInput,
				Target: targetTok.Value,
			})
			continue
		}

		if tok.Type == TokRedirectOut {
			p.advance()
			targetTok := p.advance()
			cmd.Redirections = append(cmd.Redirections, services.Redirection{
				Type:   services.RedirectOutput,
				Target: targetTok.Value,
			})
			continue
		}

		if tok.Type == TokRedirectAppend {
			p.advance()
			targetTok := p.advance()
			cmd.Redirections = append(cmd.Redirections, services.Redirection{
				Type:   services.RedirectAppend,
				Target: targetTok.Value,
			})
			continue
		}

		if tok.Type == TokRedirectErr {
			p.advance()
			targetTok := p.advance()
			cmd.Redirections = append(cmd.Redirections, services.Redirection{
				Type:   services.RedirectError,
				Target: targetTok.Value,
			})
			continue
		}

		if tok.Type == TokRedirectErrApp {
			p.advance()
			targetTok := p.advance()
			cmd.Redirections = append(cmd.Redirections, services.Redirection{
				Type:   services.RedirectErrorAppend,
				Target: targetTok.Value,
			})
			continue
		}

		if tok.Type == TokRedirectErrOut {
			p.advance()
			cmd.Redirections = append(cmd.Redirections, services.Redirection{
				Type: services.RedirectErrorToStdout,
			})
			continue
		}

		// Handle 'with' options modifier for prompts
		if tok.Type == TokWith {
			p.advance() // consume 'with'
			optTok := p.advance()
			cmd.PromptOpt = optTok.Value
			continue
		}

		// Regular token
		cmd.Tokens = append(cmd.Tokens, p.advance())
	}

	if len(cmd.Tokens) == 0 && len(cmd.Redirections) == 0 {
		return nil, nil
	}

	return cmd, nil
}

// Parse parses a full script string into ScriptNode.
func Parse(input string) (*ScriptNode, error) {
	tokens := TokenizeAll(input)
	p := NewParser(tokens)
	return p.ParseScript()
}
