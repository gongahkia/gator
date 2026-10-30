**lesson.md**

# How Base Rates Affect Fault Alerts

## Feedback on Your Answer

It is understandable to focus on the detector’s 80% rate, but **80% of alerts are not necessarily real faults**. The 80% figure means the detector flags 80% of devices that are already faulty. It does not tell us what percentage of all alerts are genuine. Those calculations use different denominators [source/primer.md].

## Worked Example: 1,000 Devices

Suppose only 10 of 1,000 devices are faulty, leaving 990 sound devices. The detector’s rates stay unchanged.

Among the faulty devices:

- True alerts: 80% of 10 = 8
- Denominator: 10 faulty devices
- Detection rate: 8 ÷ 10 = 80%

Among the sound devices:

- Expected false alerts: 5% of 990 = 49.5

The detector therefore produces about 57.5 alerts in total:

**8 true alerts + 49.5 false alerts = 57.5 alerts**

The expected percentage of alerts that represent real faults is:

**8 ÷ 57.5 × 100 ≈ 13.91%**

So about **13.91%**, not 80%, of alerts are expected to indicate real faults.

## Why the Base Rate Matters

Because faulty devices are rare, the detector checks far more sound devices. Even a small false-alert rate can then create many false alerts. Changing the fault base rate does not change the detector’s 80% detection rate or 5% false-alert rate [source/primer.md].

## Practice Question

Out of 1,000 devices, 50 are faulty. Using the same detector rates, how many true and false alerts are expected, and what percentage of all alerts represent real faults?