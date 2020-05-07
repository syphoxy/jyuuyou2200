package main

import (
	"bufio"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
)

const (
	EntryID = iota
	EntryUsage
	EntryTranslation
	EntryWord
	EntryPronunciation
	EntryDefinition
	EntryTags
	EntryEnd

	EntryDirtyMarker = byte('*')
	EntryDelimiter   = "---"
)

var ClozeDeletionRegexp = regexp.MustCompile("{{c[[:digit:]]::(.+)}}")

type EntriesParseError struct {
	line   int
	data   string
	reason string
}

func (e EntriesParseError) Data() string  { return e.data }
func (e EntriesParseError) Line() int     { return e.line }
func (e EntriesParseError) Error() string { return e.reason }

type Entry struct {
	id            int64
	dirty         bool
	comments      []string
	input         string
	usage         string
	translation   string
	word          string
	pronunciation string
	definition    string
	tags          []string
}

func (e Entry) ID() int64                { return e.id }
func (e Entry) IsDirty() bool            { return e.dirty }
func (e Entry) Comments() []string       { return e.comments }
func (e Entry) Input() string            { return e.input }
func (e Entry) Usage() string            { return e.usage }
func (e Entry) Translation() string      { return e.translation }
func (e Entry) Word() string             { return e.word }
func (e Entry) Pronunciation() string    { return e.pronunciation }
func (e Entry) Definition() string       { return e.definition }
func (e Entry) Tags() []string           { return e.tags }
func (e Entry) Audio(root string) string { return fmt.Sprintf("%s/%04d.mp3", root, e.id) }

func (e Entry) CSV(prefix, root string) []string {
	return []string{
		fmt.Sprintf("%s-%04d", prefix, e.ID()),
		e.Input(),
		e.Usage(),
		e.Translation(),
		e.Word(),
		e.Pronunciation(),
		e.Definition(),
		strings.Join(e.Tags(), ","),
		e.Audio(root),
	}
}

type Entries [2200]Entry

func (entries Entries) Write(f io.Writer, prefix, root string) (int, int, error) {
	w := csv.NewWriter(f)
	count, dirty := 0, 0
	for _, entry := range entries {
		if entry.IsDirty() {
			dirty++
		}
		if entry.ID() == 0 || entry.IsDirty() {
			continue
		}
		if err := w.Write(entry.CSV(prefix, root)); err != nil {
			return count, dirty, fmt.Errorf("failed to write csv data: %w", err)
		}
		count++
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return count, dirty, fmt.Errorf("failed to flush data: %w", err)
	}
	return count, dirty, nil
}

func NewEntriesFromFile(f io.Reader) (Entries, error) {
	const (
		digitsOffset  = 3
		dirtyOffset   = digitsOffset + 1
		commentOffset = dirtyOffset + 2
	)
	entries := Entries{}
	scanner := bufio.NewScanner(f)
	current := Entry{}
	for line := 1; scanner.Scan(); line++ {
		data := scanner.Text()
		switch (line - 1) % (EntryEnd + 1) {
		case EntryID:
			if len(data) < digitsOffset+1 {
				return entries, EntriesParseError{
					line: line,
					data: data,
					reason: fmt.Sprintf(
						"line %d: entry ID too short: %q: found %d digits, expected %d digits",
						line,
						data,
						len(data),
						digitsOffset+1,
					),
				}
			}
			id, err := strconv.ParseInt(data[:digitsOffset+1], 10, 0)
			if err != nil {
				return entries, EntriesParseError{
					line:   line,
					data:   data,
					reason: fmt.Sprintf("line %d: failed to parse entry ID: %q: %v", line, data, err),
				}
			}
			current.id = id
			current.dirty = len(data) >= dirtyOffset+1 && data[dirtyOffset] == EntryDirtyMarker
			current.comments = make([]string, 0)
			if len(data) >= commentOffset+1 {
				current.comments = append(current.comments, data[commentOffset:])
			}
		case EntryUsage:
			current.usage = data
			if matches := ClozeDeletionRegexp.FindStringSubmatch(data); matches != nil && len(matches) >= 2 {
				current.input = matches[1]
			} else {
				current.dirty = true
				current.comments = append(current.comments, "usage is missing cloze deletion.")
			}
		case EntryTranslation:
			current.translation = data
			if ClozeDeletionRegexp.FindStringSubmatch(data) == nil {
				current.dirty = true
				current.comments = append(current.comments, "translation is missing cloze deletion.")
			}
		case EntryWord:
			current.word = data
		case EntryPronunciation:
			current.pronunciation = data
		case EntryDefinition:
			current.definition = data
		case EntryTags:
			current.tags = strings.Split(data, ",")
		case EntryEnd:
			if data != EntryDelimiter {
				return entries, EntriesParseError{
					line:   line,
					data:   data,
					reason: fmt.Sprintf("line %d: unexpected end of entry. found: %q, expected: %q", line, data, EntryDelimiter),
				}
			}
			entries[current.id-1] = current
			current = Entry{}
		}
	}
	if err := scanner.Err(); err != nil {
		return entries, fmt.Errorf("failed to read file: %v", err)
	}
	return entries, nil
}

func main() {
	cli := struct {
		prefix string
		root   string
	}{}
	flag.StringVar(&cli.prefix, "p", "JLPT-N2-JY-2200", "card ID prefix")
	flag.StringVar(&cli.root, "r", "JLPT-N2-JY-2200", "media root path")
	flag.Parse()
	if len(flag.Args()) != 2 {
		log.Fatalf("invalid number of arguments: usage: %s input.txt output.csv", flag.CommandLine.Name())
	}
	i, err := os.Open(flag.Arg(0))
	if err != nil {
		log.Fatalf("failed to open input file: %s: %v", flag.Arg(0), err)
	}
	defer i.Close()
	w, err := os.Create(flag.Arg(1))
	if err != nil {
		log.Fatalf("failed to open output file: %s: %v", flag.Arg(1), err)
	}
	defer w.Close()
	entries, err := NewEntriesFromFile(i)
	if err != nil {
		log.Fatalf("failed to process input file: %v", err)
	}
	count, dirty, err := entries.Write(w, cli.prefix, cli.root)
	if err != nil {
		log.Fatalf("failed to write output file: %v", err)
	}
	if dirty != 0 {
		fmt.Println("found", dirty, "dirty entries.")
		for _, entry := range entries {
			if !entry.IsDirty() {
				continue
			}
			fmt.Print("\n")
			if len(entry.Comments()) == 0 {
				fmt.Printf("  %04d: marked.\n", entry.ID())
				continue
			}
			for index, comment := range entry.Comments() {
				if index == 0 {
					fmt.Printf("  %04d: %s\n", entry.ID(), comment)
				} else {
					fmt.Println(strings.Repeat(" ", 2+4+1), comment)
				}
			}
		}
		fmt.Print("\n")
	}
	fmt.Println("generated", count, "entries.")
}