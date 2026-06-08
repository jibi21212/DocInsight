package chunker

import (
	"regexp"
	"strings"

	"github.com/docinsight/backend/internal/model"
	"github.com/docinsight/backend/internal/pdf"
	"github.com/google/uuid"
)

var (
	// Go's RE2 regexp doesn't support lookbehinds, so we match the punctuation
	// plus trailing whitespace and re-append the punctuation in splitIntoSentences.
	sentenceEndings = regexp.MustCompile(`([.!?])\s+`)
	paragraphBreak  = regexp.MustCompile(`\n\s*\n`)
)

type TextChunk struct {
	Content    string
	PageNumber int
	ChunkIndex int
	Metadata   model.ChunkMetadata
}

type taggedParagraph struct {
	text       string
	pageNumber int
}

func CreateChunks(pages []pdf.Page, chunkSize, chunkOverlap int) []TextChunk {
	if len(pages) == 0 {
		return nil
	}

	var tagged []taggedParagraph
	for _, page := range pages {
		paragraphs := splitIntoParagraphs(page.Text)
		for _, p := range paragraphs {
			tagged = append(tagged, taggedParagraph{text: p, pageNumber: page.Number})
		}
	}

	if len(tagged) == 0 {
		return nil
	}

	var chunks []TextChunk
	chunkIndex := 0

	var currentParts []string
	currentLength := 0
	currentStartPage := tagged[0].pageNumber
	currentEndPage := currentStartPage

	for _, tp := range tagged {
		// If a single paragraph exceeds chunk size, split by sentences
		if len(tp.text) > chunkSize {
			// Flush current buffer
			if len(currentParts) > 0 {
				content := strings.Join(currentParts, "\n\n")
				chunks = append(chunks, buildChunk(content, currentStartPage, currentEndPage, chunkIndex))
				chunkIndex++

				overlapParts := getOverlapParts(currentParts, chunkOverlap)
				currentParts = overlapParts
				currentLength = joinLength(overlapParts, "\n\n")
				currentStartPage = currentEndPage
			}

			// Split large paragraph into sentence-level chunks
			sentences := splitIntoSentences(tp.text)
			var sentBuf []string
			sentLen := 0

			for _, sent := range sentences {
				if sentLen+len(sent) > chunkSize && len(sentBuf) > 0 {
					content := strings.Join(sentBuf, " ")
					chunks = append(chunks, buildChunk(content, tp.pageNumber, tp.pageNumber, chunkIndex))
					chunkIndex++

					overlapSentences := getOverlapParts(sentBuf, chunkOverlap)
					sentBuf = overlapSentences
					sentLen = joinLength(overlapSentences, " ")
				}
				sentBuf = append(sentBuf, sent)
				sentLen += len(sent) + 1
			}

			if len(sentBuf) > 0 {
				currentParts = []string{strings.Join(sentBuf, " ")}
				currentLength = len(currentParts[0])
				currentStartPage = tp.pageNumber
				currentEndPage = tp.pageNumber
			}
			continue
		}

		// Normal case: accumulate paragraphs
		if currentLength+len(tp.text)+2 > chunkSize && len(currentParts) > 0 {
			content := strings.Join(currentParts, "\n\n")
			chunks = append(chunks, buildChunk(content, currentStartPage, currentEndPage, chunkIndex))
			chunkIndex++

			overlapParts := getOverlapParts(currentParts, chunkOverlap)
			currentParts = overlapParts
			currentLength = joinLength(overlapParts, "\n\n")
			currentStartPage = currentEndPage
		}

		currentParts = append(currentParts, tp.text)
		currentLength += len(tp.text) + 2
		currentEndPage = tp.pageNumber
	}

	// Flush remaining
	if len(currentParts) > 0 {
		content := strings.Join(currentParts, "\n\n")
		chunks = append(chunks, buildChunk(content, currentStartPage, currentEndPage, chunkIndex))
	}

	return chunks
}

func ToModelChunks(textChunks []TextChunk, documentID uuid.UUID) []model.Chunk {
	chunks := make([]model.Chunk, len(textChunks))
	for i, tc := range textChunks {
		chunks[i] = model.Chunk{
			ID:         uuid.New(),
			DocumentID: documentID,
			Content:    tc.Content,
			PageNumber: tc.PageNumber,
			ChunkIndex: tc.ChunkIndex,
			Metadata:   tc.Metadata,
		}
	}
	return chunks
}

func buildChunk(content string, startPage, endPage, chunkIndex int) TextChunk {
	words := strings.Fields(content)
	return TextChunk{
		Content:    content,
		PageNumber: startPage,
		ChunkIndex: chunkIndex,
		Metadata: model.ChunkMetadata{
			CharCount: len(content),
			WordCount: len(words),
			StartPage: startPage,
			EndPage:   endPage,
		},
	}
}

func splitIntoParagraphs(text string) []string {
	parts := paragraphBreak.Split(text, -1)
	var result []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func splitIntoSentences(text string) []string {
	// Find all sentence-ending punctuation + whitespace boundaries
	matches := sentenceEndings.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			return nil
		}
		return []string{trimmed}
	}

	var result []string
	prev := 0
	for _, m := range matches {
		// Include the punctuation character (m[0] is start of "[.!?]\s+")
		// The sentence runs from prev to m[0]+1 (include the punctuation char)
		sentence := strings.TrimSpace(text[prev : m[0]+1])
		if sentence != "" {
			result = append(result, sentence)
		}
		prev = m[1] // after the whitespace
	}
	// Remaining text after last match
	if prev < len(text) {
		trimmed := strings.TrimSpace(text[prev:])
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func getOverlapParts(parts []string, overlapSize int) []string {
	var result []string
	totalLen := 0

	for i := len(parts) - 1; i >= 0; i-- {
		if totalLen >= overlapSize {
			break
		}
		result = append([]string{parts[i]}, result...)
		totalLen += len(parts[i])
	}

	return result
}

func joinLength(parts []string, sep string) int {
	if len(parts) == 0 {
		return 0
	}
	total := 0
	for _, p := range parts {
		total += len(p)
	}
	total += len(sep) * (len(parts) - 1)
	return total
}
