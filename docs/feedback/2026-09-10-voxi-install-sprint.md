# Voxi Install Sprint Feedback

The live privileged validation caught a real installer integration issue that unit
tests did not: piping a systemd unit through `sudo install /dev/stdin` failed after
password entry. Staging the unit in the user cache and passing its path to `sudo
install` made the privilege boundary reliable.

The sprint also exposed a review-process constraint: the agent-thread limit prevented
a second independent reviewer after the development fixes. Local diff review and the
successful user-scoped and privileged live checks completed the remaining gate.
