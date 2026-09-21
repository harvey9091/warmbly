// The API accepts 0 (follow the instance setting) or 3 to 3650 days for how
// long warmup mail stays in a mailbox. The stepper moves by one, so stepping
// down from 3 lands on 0 and stepping up from 0 lands on 3: 1 and 2, which the
// API refuses, can never be sent.
export function clampWarmupRetentionDays(n: number): number {
    if (!Number.isFinite(n) || n <= 0) return 0;
    const whole = Math.floor(n);
    if (whole < 3) return whole === 1 ? 0 : 3;
    return Math.min(3650, whole);
}
