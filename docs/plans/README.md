# Plans Lifecycle

Plans in this directory are execution artifacts, not standing instructions. A
plan is live only when its status says `active`; otherwise read it as historical
context.

Every plan, log, or handoff file in `docs/plans/` must start with a status
block immediately after the title:

```markdown
> **Status:** draft | active | executed | abandoned -- short reason.
> **Landing:** pending | YYYY-MM-DD, commit `abcdef0` | not applicable -- reason.
> **Disposition:** commit with work | retained historical artifact | delete at land time.
```

- `draft` means the plan is being shaped and must not be executed without an
  explicit current instruction.
- `active` means the plan is approved live work. It must name the tracker issue
  or expedition that owns it.
- `executed` means the work landed or otherwise completed. The status block must
  name the landing commit when the historical record has one.
- `abandoned` means the plan was superseded or rejected. The status block must
  link the replacement plan or explain why no replacement exists.

Retention policy:

- Commit active plans with the work they support when they contain durable
  design rationale, handoff context, or verification evidence.
- Delete plans at land time when they were only temporary execution notes.
- Retain executed or abandoned plans only as historical artifacts, with a status
  block that prevents future agents from treating old imperative steps as live
  work.
- Do not leave untracked plan files behind. At the end of a task, every plan
  file should be committed with a lifecycle status or deleted deliberately.
