# U0 — Notion body-range overwrite containment and update-path repair

Scope: incident `BODY_RANGE_OVERWRITE-20261011T2334Z`; eleven production pages.
This urgent operations sprint is separate from external management API work and
from Sori PR39's ledger tail-loss defect.

| Work | Acceptance | State |
| --- | --- | --- |
| Prevent unsafe body selection | Generated exact literals only; no whole-page replacement; ownership and size checks | Code implemented |
| Save and verify protected content | Durable preimage before mutation; exact whole-body/property readback | Code implemented |
| Stop propagation | No page two after unexpected readback, lost ACK or async failure; state survives restart | Regression verified |
| Connect actual caller | Hosted Code Mode runner uses same authenticated connector and remote broker | Canary verification required |
| Restore damaged pages | Recover lost historical ranges and retain newest genuine worker receipts | Pending |
| Resume central safely | Restored pages reconciled; guarded calling path observed; production hold reviewed | Blocked |

Confirmed cause: model-built selection was not constrained by executable range
checks or a per-page preservation barrier. Exact underlying selection-calculation
bug is unconfirmed because the failed turn's executable is unavailable here.
Existing preservation/readback instructions alone had already failed.

The supervisor remains disabled. Queue Watch remains read-only. No product code,
worker session, merge, deploy, timer or usage gate is changed by this sprint.
Raw-tool capability isolation is unavailable in the current hosted task settings;
the code path is enforced within the runner, not claimed as a platform-wide ACL.
