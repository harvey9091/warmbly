package advisor

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/google/uuid"

	"github.com/warmbly/warmbly/internal/app/copyjudge"
	"github.com/warmbly/warmbly/internal/repository"
)

// maxCopyJudgmentsPerRun bounds how many fresh TypeSafe calls one evaluation
// may spend. Verdicts are cached by content hash, so this only bites on a
// workspace's first run or after a wave of rewrites, and the next run judges
// the rest.
const maxCopyJudgmentsPerRun = 40

// copyJudgeTimeout bounds one judgment.
const copyJudgeTimeout = 10 * time.Second

// judgeCopy fills snapshot.CopyJudgments for every email step in a non-draft
// campaign, from the cache where it can and from TypeSafe where it must. It
// never fails the evaluation: a judge that is down leaves steps unjudged, and
// the detectors that read a verdict stay silent on those.
func (s *service) judgeCopy(ctx context.Context, snapshot *repository.AdvisorSnapshot) {
	if s.judge == nil || s.judgeCache == nil {
		return
	}
	steps := emailSteps(snapshot)
	if len(steps) == 0 {
		return
	}
	verdicts := make(map[uuid.UUID]copyjudge.Verdict, len(steps))
	fresh := 0
	capped := false
	for _, sc := range steps {
		body := copyjudge.Body(sc.step.BodyPlain, sc.step.BodyHTML)
		hash := copyjudge.ContentHash(sc.step.Subject, body)

		cached, err := s.judgeCache.Get(ctx, snapshot.OrganizationID, hash)
		if err != nil {
			log.Printf("advisor: copy judgment cache read for org %s: %v", snapshot.OrganizationID, err)
		}
		if cached != nil {
			verdicts[sc.step.ID] = *cached
			continue
		}

		if fresh >= maxCopyJudgmentsPerRun {
			capped = true
			continue
		}
		fresh++
		// Bounded per call, and the first failure ends the run's judging:
		// Evaluate runs inside the refresh request, and a judge that is down
		// must not be asked forty times.
		jctx, cancel := context.WithTimeout(ctx, copyJudgeTimeout)
		v, err := copyjudge.Judge(jctx, s.judge, sc.step.Subject, body)
		cancel()
		if err != nil {
			log.Printf("advisor: copy judgment for step %s in org %s: %v", sc.step.ID, snapshot.OrganizationID, err)
			if !errors.Is(err, copyjudge.ErrNoContent) {
				break
			}
			continue
		}
		if err := s.judgeCache.Put(ctx, snapshot.OrganizationID, hash, v); err != nil {
			log.Printf("advisor: copy judgment cache write for org %s: %v", snapshot.OrganizationID, err)
		}
		verdicts[sc.step.ID] = *v
	}
	if capped {
		log.Printf("advisor: copy judgment cap of %d reached for org %s; the rest are judged next run", maxCopyJudgmentsPerRun, snapshot.OrganizationID)
	}
	snapshot.CopyJudgments = verdicts
}
