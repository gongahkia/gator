# Base Rates and Fault Alerts

## Feedback on your answer

You correctly noticed that **80%** describes the detector’s success with faulty devices. However, it does **not** mean that 80% of alerts are real faults. Those questions use different denominators. This is an easy mix-up, and separating the device groups makes the difference clear.

## Work from the 1,000 devices

There are 20 faulty devices and 980 sound devices.

- The detector flags 80% of the faulty devices: 20 × 0.80 = **16 true alerts**.
- It incorrectly flags 5% of the sound devices: 980 × 0.05 = **49 false alerts**.
- Altogether, it produces 16 + 49 = **65 alerts**.

To find the percentage of alerts that are real faults, divide true alerts by all alerts:

**16 ÷ 65 × 100 = 24.62%** (approximately).

So about **24.62% of alerts are real faults**, not 80%. Although the detector catches most faults, sound devices are far more common, so their 5% false-alert rate creates many false alerts. This is a fictional example, not observed field data. [Source: `primer.md`, lines 3–12]

## Two denominators

The detection rate is **16/20 = 80%**; its denominator is all **faulty devices**. Alert reliability is **16/65 ≈ 24.62%**; its denominator is **all alerted devices**.

## Fresh practice question

In 1,000 devices, 40 are faulty and 960 are sound. A detector flags 75% of faulty devices and 5% of sound devices. How many alerts are real, how many are false, and what percentage of all alerts are real faults?
