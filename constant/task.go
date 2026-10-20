package constant

type TaskPlatform string

const (
	TaskPlatformSuno       TaskPlatform = "suno"
	TaskPlatformMidjourney              = "mj"
)

// MaxVideoTaskRequestBodyBytes bounds each independently persisted request
// snapshot. Keeping the limit in constant avoids coupling provider adaptors to
// the model package, which imports relay/common for task utilities.
const MaxVideoTaskRequestBodyBytes = 2 * 1024 * 1024

const (
	TaskActionImageToVideo     = "image_to_video"
	TaskActionTextToVideo      = "text_to_video"
	TaskActionFirstTailToVideo = "first_tail_to_video"
	TaskActionReferenceToVideo = "reference_to_video"
	TaskActionRemix            = "remix"
)

// Deprecated: legacy action names kept as compile-time aliases for downstream
// code that still emits the pre-canonical vocabulary. New code must use the
// canonical TaskAction* values; persisted rows are migrated by
// NormalizeTaskAction when read.
const (
	SunoActionMusic  = "MUSIC"
	SunoActionLyrics = "LYRICS"

	TaskActionGenerate          = TaskActionImageToVideo
	TaskActionTextGenerate      = TaskActionTextToVideo
	TaskActionFirstTailGenerate = TaskActionFirstTailToVideo
	TaskActionReferenceGenerate = TaskActionReferenceToVideo
)

var legacyTaskActionAliases = map[string]string{
	"generate":          TaskActionImageToVideo,
	"textGenerate":      TaskActionTextToVideo,
	"firstTailGenerate": TaskActionFirstTailToVideo,
	"referenceGenerate": TaskActionReferenceToVideo,
	"remixGenerate":     TaskActionRemix,
}

// TaskPluginEnabled is the master switch for the whole task-plugin system.
// When disabled, factory and override plugins both stop serving.
var TaskPluginEnabled = true

// TaskPluginOverrideEnabled controls whether the database override layer is
// active. When disabled, uploaded plugins are ignored and factory plugins are
// used instead; the factory layer is unaffected.
var TaskPluginOverrideEnabled = true

// TaskActionSpellings returns every persisted spelling of action: the
// canonical name plus any legacy alias that normalizes to it.
func TaskActionSpellings(action string) []string {
	canonical := NormalizeTaskAction(action)
	spellings := []string{canonical}
	for legacy, target := range legacyTaskActionAliases {
		if target == canonical {
			spellings = append(spellings, legacy)
		}
	}
	return spellings
}

// NormalizeTaskAction maps persisted legacy action names to the canonical task
// action vocabulary. Unknown platform-specific actions pass through unchanged.
func NormalizeTaskAction(action string) string {
	if canonical, ok := legacyTaskActionAliases[action]; ok {
		return canonical
	}
	return action
}

func ShouldStoreVideoTaskRequestBody(channelType int) bool {
	switch channelType {
	case ChannelTypeAli, ChannelTypeDoubaoVideo, ChannelTypeVolcNative:
		return true
	default:
		return false
	}
}
