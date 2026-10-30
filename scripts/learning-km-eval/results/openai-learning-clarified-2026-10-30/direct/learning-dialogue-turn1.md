**lesson.md**

# How Base Rates Affect Fault Alerts

## Your Answer

Your reasoning uses a real detector statistic, but **80% is not the percentage of alerts that are real faults**. It means the detector flags 80% of devices that are already faulty. The denominator is the 20 faulty devices—not all alerts.

This denominator mix-up is common. To find how trustworthy an alert is, we must count both true and false alerts.

## Worked Example: 1,000 Devices

Among 1,000 devices:

- **20 are faulty.** The detector flags 80%, producing  
  \(20 \times 0.80 = 16\) true alerts.
- **980 are sound.** The detector incorrectly flags 5%, producing  
  \(980 \times 0.05 = 49\) false alerts.

There are therefore:

\[
16 + 49 = 65 \text{ total alerts}
\]

Of these alerts, 16 represent real faults:

\[
\frac{16}{65}\times 100 \approx 24.62\%
\]

So **about 24.62% of alerts are real faults**, not 80%. Faults are rare, so the large sound-device group produces more false alerts than the faulty group produces true alerts. This is a fictional example rather than field data ([source/primer.md](source/primer.md)).

## Distinguish the Denominators

- **Detection rate:** \(16/20 = 80\%\), using all faulty devices.
- **Real-fault share of alerts:** \(16/65 \approx 24.62\%\), using all alerts.

## Check Your Understanding

In another group, a detector produces 30 true alerts and 10 false alerts. What percentage of all alerts correspond to real faults, and what denominator should you use?