package agents

// OutcomeBucket classifies agent run outcomes.
type OutcomeBucket int

const (
	BucketSilent OutcomeBucket = iota // ran, no changes needed
	BucketActed                       // ran, made changes (created PR, pushed fix)
	BucketAlert                       // ran, needs human attention (failed, errored)
)

func (b OutcomeBucket) String() string {
	switch b {
	case BucketActed:
		return "acted"
	case BucketAlert:
		return "alert"
	default:
		return "silent"
	}
}

// TriggerType describes how an agent run was triggered.
type TriggerType int

const (
	TriggerCron   TriggerType = iota // scheduled
	TriggerEvent                     // GitHub event (push, PR)
	TriggerManual                    // user-dispatched
)

func (t TriggerType) String() string {
	switch t {
	case TriggerEvent:
		return "event"
	case TriggerManual:
		return "manual"
	default:
		return "cron"
	}
}

// ClassifyOutcome determines the outcome bucket for a workflow run.
func ClassifyOutcome(conclusion string, artifactCount int) OutcomeBucket {
	switch conclusion {
	case "failure", "timed_out":
		return BucketAlert
	case "success":
		if artifactCount > 0 {
			return BucketActed
		}
		return BucketSilent
	case "cancelled":
		return BucketSilent
	default:
		return BucketSilent
	}
}

// ClassifyTrigger determines how a run was triggered.
func ClassifyTrigger(event string) TriggerType {
	switch event {
	case "schedule":
		return TriggerCron
	case "workflow_dispatch":
		return TriggerManual
	default:
		return TriggerEvent
	}
}
