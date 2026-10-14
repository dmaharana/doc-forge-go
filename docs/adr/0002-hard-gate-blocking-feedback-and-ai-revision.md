# Hard-Gated Blocking Feedback and Iterative AI Document Revision

We enforced a strict gate where documents cannot transition to the `approved` state while any feedback marked `is_blocking = 1` remains in `open` status. Rather than relying on unstructured inline comments or advisory warnings, reviewers record categorized feedback that triggers an automated LLM revision loop. The revision synthesizes a new immutable document version and automatically advances open feedback to `addressed`, preserving an auditable technical sign-off trail before publication to Confluence.
