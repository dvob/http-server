package config

import (
	"fmt"
	"strings"
	"unicode"
)

func ParseArgs(args []string) (map[string][]HandlerConfig, error) {
	config := []byte(strings.Join(args, " "))
	return Parse(config)
}

func Parse(input []byte) (map[string][]HandlerConfig, error) {
	p := &parser{
		input: input,
		pos:   0,
	}
	return p.parse()
}

type parser struct {
	input []byte
	pos   int
}

func (p *parser) peek() (byte, bool) {
	if p.pos >= len(p.input) {
		return 0, false
	}
	return p.input[p.pos], true
}

func (p *parser) consume(chr byte) error {
	nextChr, ok := p.peek()
	if !ok {
		return fmt.Errorf("unexpected EOF. expected '%c'", chr)
	}
	if nextChr != chr {
		return fmt.Errorf("unexpected '%c' at offset %d expected '%c'", nextChr, p.pos+1, chr)
	}
	p.next()
	return nil
}

func (p *parser) next() {
	p.pos++
}

func (p *parser) skipSpace() {
	for {
		c, ok := p.peek()
		if !ok {
			break
		}
		if c == ' ' || c == '\n' || c == '\t' {
			p.next()
			continue
		}
		break
	}
}

func (p *parser) readQuotedString() (string, error) {
	err := p.consume('"')
	if err != nil {
		return "", nil
	}

	start := p.pos

	for {
		c, ok := p.peek()
		if !ok {
			return "", fmt.Errorf("unexpected EOF. end of string not found")
		}

		if c == '"' {
			p.next()
			break
		}
		p.next()
	}
	return string(p.input[start : p.pos-1]), nil
}

func (p *parser) readNakedString() (string, error) {
	start := p.pos
	for {
		c, ok := p.peek()
		if !ok {
			break
		}
		if unicode.IsSpace(rune(c)) {
			break
		}
		if c == ',' || c == '}' || c == '{' || c == ':' {
			break
		}
		p.next()

	}
	return string(p.input[start:p.pos]), nil
}

func (p *parser) readWord() (string, error) {
	c, ok := p.peek()
	if !ok {
		return "", fmt.Errorf("unexpected EOF. expected word")
	}
	if c == '"' {
		v, err := p.readQuotedString()
		return v, err
	}
	v, err := p.readNakedString()
	return v, err
}

func (p *parser) parseSettings() (map[string]string, error) {
	result := map[string]string{}

	err := p.consume('{')
	if err != nil {
		return nil, err
	}

	p.skipSpace()

	for {
		key, err := p.readWord()
		if err != nil {
			return nil, fmt.Errorf("failed to read key: %w", err)
		}

		err = p.consume(':')
		if err != nil {
			return nil, err
		}

		p.skipSpace()
		value, err := p.readWord()
		if err != nil {
			return nil, fmt.Errorf("failed to read value: %w", err)
		}
		result[key] = value

		p.skipSpace()

		c, ok := p.peek()
		if !ok {
			return nil, fmt.Errorf("expected ',' or '}' at offset %d, got '%c'", p.pos+1, c)
		}
		if c == '}' {
			p.next()
			break
		}
		if c == ',' {
			p.next()
			p.skipSpace()
			continue
		}
		return nil, fmt.Errorf("unexpected '%c' at offset %d", c, p.pos+1)
	}
	return result, nil
}

type HandlerConfig struct {
	Name     string
	Settings map[string]string
}

func (p *parser) parse() (map[string][]HandlerConfig, error) {
	mappings := map[string][]HandlerConfig{}
	currentPath := "/"
	for {

		p.skipSpace()

		if _, ok := p.peek(); !ok {
			break
		}

		word, err := p.readWord()
		if err != nil {
			return nil, err
		}

		// path
		if strings.HasPrefix(word, "/") {
			currentPath = word
			err := p.consume(':')
			if err != nil {
				return nil, fmt.Errorf("missing ':' after path '%s' at %d", word, p.pos+1)
			}
			continue
		}

		// handler / middleware
		config := HandlerConfig{
			Name: word,
		}

		c, ok := p.peek()
		if c == '{' {
			settings, err := p.parseSettings()
			if err != nil {
				return nil, err
			}
			config.Settings = settings
		}

		mappings[currentPath] = append(mappings[currentPath], config)
		if !ok {
			break
		}
	}
	return mappings, nil
}
