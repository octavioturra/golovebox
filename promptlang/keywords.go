// Package promptlang implements the golovebox prompt DSL parser.
package promptlang

// KeywordType identifies a semantic annotation keyword in a spec file.
type KeywordType string

const (
	KwAttentionHere KeywordType = "ATTENTION_HERE"
	KwNotifyMe      KeywordType = "NOTIFY_ME"
	KwNoRetry       KeywordType = "NO_RETRY"
	KwTry           KeywordType = "TRY"
	KwOrElse        KeywordType = "OR_ELSE"
	KwNotTodo       KeywordType = "NOT_TODO"
	KwPauseToReview KeywordType = "PAUSE_TO_REVIEW"
	KwRunTest       KeywordType = "RUN_TEST"
	KwWhen          KeywordType = "WHEN"
	KwDo            KeywordType = "DO"
	KwCommit        KeywordType = "COMMIT"
)

// CheckpointKeywords marks keywords that produce a checkpoint (human-gate) DAG node.
var CheckpointKeywords = map[KeywordType]bool{
	KwAttentionHere: true,
	KwPauseToReview: true,
}

// allKeywords lists every keyword in longest-first order to avoid prefix collisions.
// Multi-word keywords must appear before any of their constituent words.
var allKeywords = []KeywordType{
	KwAttentionHere,
	KwPauseToReview,
	KwNotifyMe,
	KwNoRetry,
	KwNotTodo,
	KwRunTest,
	KwOrElse,
	KwCommit,
	KwWhen,
	KwTry,
	KwDo,
}
