# Column approval in chat

Add `approve-columns CONFIG_SHA256` for configs without word checks. Review supplies the
exact config hash. The agent must show all rules and risks, wait for explicit human approval,
and then pass that hash. Reuse the existing approval storage and config invalidation.
Refuse configs requiring word review before reading raw files or private words. Keep ordinary
`approve` terminal-only. Update hooks, setup instructions and docs; test stale hashes,
word-check refusal, secret-free output and scrub after approval.
