# Bot Architecture

## Components

-   Bot Manager
-   Strategy
-   Evaluator
-   Command Generator

## Lifecycle

Turn starts → Bot selected → Legal actions generated → Strategy scores
actions → Engine validates.

## Responsibilities

Engine: - legality - state transitions

Bot: - action selection - timing - personality

Never duplicate game rules outside the engine.
