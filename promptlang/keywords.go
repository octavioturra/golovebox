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
	KwNewBranch     KeywordType = "NEW BRANCH"
	KwPush          KeywordType = "PUSH"
	KwPR            KeywordType = "PR"
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
	KwNewBranch, // "NEW BRANCH" before single-word keywords
	KwNotifyMe,
	KwNoRetry,
	KwNotTodo,
	KwRunTest,
	KwOrElse,
	KwWhen,
	KwPush,
	KwTry,
	KwDo,
	KwPR,
}
