# Why Base Rates Matter for Fault Alerts

## Feedback on your answer
Your answer uses the detector’s 80% rate, which is an understandable starting point. However, **80% is not the percentage of alerts that are real faults**. It answers a different question: “Of the devices that really are faulty, what percentage receive an alert?” To judge an alert, we must also count alerts on sound devices.

## Worked example: 1,000 devices
Begin with the two groups:

- **20 faulty devices:** 80% are flagged, giving **16 true alerts**.
- **980 sound devices:** 5% are incorrectly flagged, giving **49 false alerts**.

That makes **65 total alerts**: 16 true plus 49 false. Therefore,

**percentage of alerts that are real faults = 16 ÷ 65 × 100 ≈ 24.62%**.

So roughly one in four alerts—not 80%—corresponds to a real fault. This happens because faults are rare: the small false-alert rate is applied to a much larger group of sound devices. The figures are fictional teaching data, not field observations.

## Keep the denominators clear
The **80% detection rate** is **16/20**; its denominator is all faulty devices. The **24.62% probability after an alert** is **16/65**; its denominator is all alerts. Changing the denominator changes the question being answered.

## Fresh practice question
Suppose 1,000 devices include 100 faulty and 900 sound devices. The detector still flags 80% of faulty devices and 5% of sound devices. How many true and false alerts occur, and what percentage of all alerts are real faults?

*Source: `primer.md`, lines 3–12.*
