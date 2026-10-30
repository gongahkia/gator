**lesson.md**

# How Base Rates Affect Fault Alerts

## Start with the Base Rate

A base rate is the proportion of devices that are actually faulty before testing. When faults are rare, even a fairly accurate detector can produce many false alerts because it examines many more sound devices than faulty ones.

## Worked Example: 1,000 Devices

Suppose 20 of 1,000 devices are faulty, while 980 are sound. The detector flags 80% of faulty devices:

- **True alerts:** 80% of 20 = 16  
- **Missed faults:** 20 − 16 = 4  

It also incorrectly flags 5% of sound devices:

- **False alerts:** 5% of 980 = 49  
- **Correct non-alerts:** 980 − 49 = 931  

There are therefore 16 + 49 = 65 total alerts. Of these, only 16 indicate real faults:

**16 ÷ 65 ≈ 24.62%**

Thus, a flagged device has about a 24.62% chance of actually being faulty in this fictional example [source/primer.md].

## Keep the Denominators Straight

The **80% detection rate** is 16 true alerts divided by **20 faulty devices**. Its denominator is all faulty devices.

The **24.62% alert reliability** is 16 true alerts divided by **65 total alerts**. Its denominator is all alerts.

Confusing these denominators incorrectly turns “80% of faults are detected” into “80% of alerts are faults.” The low fault base rate explains why those statements differ.

## Check Your Understanding

If only 10 of 1,000 devices were faulty, while both detector rates stayed unchanged, would the percentage of alerts that represent real faults increase or decrease? Explain using the two denominators.