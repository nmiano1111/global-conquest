# Using the Lux Delux AIs for Global Conquest Research

## Summary

The paper **"Using Graph Convolutional Networks and TD(λ) to Play the
Game of Risk"** generated its training data using a diverse collection
of built-in AI opponents from **Lux Delux**, rather than a single
self-play policy.

This is directly relevant to Global Conquest because the biggest risk of
self-play is **policy monoculture**: if every game is played by the same
heuristic, the training dataset only contains states that heuristic
naturally visits.

The Lux approach intentionally diversified the training distribution.

------------------------------------------------------------------------

# The AIs Used in the Paper

The paper generated approximately:

-   2,000 games
-   \~200,000 turn-end states

using six built-in Lux AI agents:

-   Angry
-   Pixie
-   Cluster
-   Quo
-   Killbot
-   Boscoe

The authors selected these agents because they represented different
strengths and strategic styles.

## Paper

**Using Graph Convolutional Networks and TD(λ) to Play the Game of
Risk**

https://arxiv.org/abs/2009.06355

------------------------------------------------------------------------

# Can We Access These AIs?

Yes.

Sillysoft provides an official SDK that includes:

-   Java API
-   Documentation
-   Source code for the bundled Lux AIs
-   Example agents

SDK:

https://sillysoft.net/sdk/

Direct SDK download:

https://sillysoft.net/sdk/SillysoftSDK.zip

Lux Delux:

https://sillysoft.net/lux/

------------------------------------------------------------------------

# Should We Use Lux Directly?

Probably not.

Global Conquest already has:

-   an authoritative Go engine
-   deterministic simulation
-   a tournament runner
-   strategy interfaces
-   training-data generation

Running Lux externally would introduce:

-   Java integration
-   different rules
-   different APIs
-   map differences
-   card-rule differences
-   synchronization complexity

Instead, Lux should primarily be viewed as a **source of strategic
diversity**.

------------------------------------------------------------------------

# Recommended Approach

Rather than attempting to embed Lux into Global Conquest:

1.  Download the SDK.
2.  Study each AI implementation.
3.  Document its strategic principles.
4.  Implement equivalent strategies in Go.
5.  Run mixed tournaments using those strategies.
6.  Capture training data using the existing tournament framework.

Conceptually:

``` text
Lux AI source
        ↓
Understand strategic ideas
        ↓
Native Go implementation
        ↓
Mixed-policy tournaments
        ↓
Training dataset
        ↓
GCN / Monte Carlo / ML
```

------------------------------------------------------------------------

# Why This Is Better

Advantages:

-   Uses the existing authoritative engine.
-   Produces perfectly rule-compatible training data.
-   Supports deterministic seeds.
-   Supports existing tracing.
-   Works with tournament automation.
-   Easily scales to hundreds of thousands of games.

------------------------------------------------------------------------

# Future Strategy Population

Instead of only `scored-v1`, create a diverse ecosystem.

Possible strategy families:

-   scored-v1
-   scored-v1 (perturbed weights)
-   aggressive
-   defensive
-   continent-focused
-   elimination-focused
-   card-focused
-   Lux Angry-inspired
-   Lux Pixie-inspired
-   Lux Cluster-inspired
-   Lux Quo-inspired
-   Lux Killbot-inspired
-   Lux Boscoe-inspired
-   Monte Carlo
-   GCN evaluator
-   Future learned models

This creates a much healthier training distribution.

------------------------------------------------------------------------

# Long-Term Research Loop

``` text
Study external AI designs
        ↓
Implement native Go variants
        ↓
Run headless tournaments
        ↓
Generate datasets
        ↓
Train improved models
        ↓
Create stronger strategies
        ↓
Repeat
```

The existing headless tournament runner becomes the primary
data-generation engine, while Lux serves as a source of ideas and
strategic diversity rather than the runtime used for production
experiments.

------------------------------------------------------------------------

# Licensing Note

The SDK includes source code, but source availability does **not**
automatically grant unrestricted reuse.

Before copying code directly into Global Conquest:

-   Review the SDK license.
-   Review Sillysoft's terms.
-   Prefer clean-room reimplementations of strategic ideas over copying
    Java code.

This minimizes legal risk while preserving the benefits of the research.
