# Family Walks

A parent opens this most evenings (or whenever they remember) to mark
whether the family took a walk that night and see their streak.

## Records

**WalkDay**
- date (unique)
- walked (yes/no)
- note (optional text)
- created_at / updated_at

## Views

- `GET /` — Today's yes/no toggle, current streak count, milestone message
  when hit
- `GET /history` — Simple list/grid of past days, tap any day to log/edit
  yes/no + note

## Platform surfaces

- Dashboard card: today's status + current streak
- Chat tools: log today's walk (yes/no), check current streak
- Intents: "log tonight's walk", "what's our streak"

## Streak rules

- A "no" resets the streak to zero.
- An unlogged day is ignored — it neither extends nor breaks a streak that
  was built on prior consecutive "yes" days (checked by walking backward
  from the most recent logged "yes"/"no" day, skipping gaps of unlogged
  days entirely).
- The streak counts only contiguous logged "yes" days looking back from
  today (or from the most recent "yes" if today is unlogged), stopping the
  moment a logged "no" is hit.

## Milestones

Celebrate lightly at streaks of 3, 7, 14, 30, 60, 100 nights (and every
100 after that).

## Non-goals

- No sharing/export.
- No multiple entries per day.
- No grace periods or forgiveness logic beyond the unlogged-day rule
  above.

## Later

- Track who joined (kids, dog).
- Weekly/monthly stats view.
