-- Preserve legacy steps while allowing v2 inspection templates to be authored.
-- Runtime availability is checked separately before publication.
alter table task_steps drop constraint task_steps_uses_valid;
alter table task_steps add constraint task_steps_uses_valid check(uses in (
  'device.command','device.collect','algorithm.run','issue.create-or-update',
  'copilot.run','report.generate','inspection.observe','inspection.detect'
));
