
# Global Conquest – Monte Carlo + Evaluator Strategy Roadmap

> This roadmap describes a staged path from heuristic play to Monte Carlo search and eventually MCTS using a pluggable learned evaluator.

## Primary References

### Risk-specific

1. **An Automated Technique for Drafting Territories in the Board Game Risk** (AIIDE 2010)
   - https://ojs.aaai.org/index.php/AIIDE/article/view/12388
   - Demonstrates Monte Carlo Tree Search combined with learned evaluation for Risk drafting.

2. **Using Graph Convolutional Networks and TD(λ) to Play the Game of Risk**
   - https://arxiv.org/abs/2009.06355
   - Shows how a learned evaluator complements search.

### General Search

3. **Monte Carlo Tree Search**
   - https://en.wikipedia.org/wiki/Monte_Carlo_tree_search

4. **Bandit Based Monte-Carlo Planning (UCT)**
   - http://www.sztaki.hu/~szcsaba/papers/ecml06.pdf

5. **AlphaGo Nature Paper**
   - https://www.nature.com/articles/nature16961
   - Canonical example of combining search with neural evaluation.

## Recommended Progression

```text
Heuristic
    ↓
Flat Monte Carlo
    ↓
Learned Evaluator
    ↓
Better Rollout Policies
    ↓
Progressive Widening
    ↓
Tree Search
    ↓
MCTS
    ↓
Search Distillation
```

## Major Milestones

1. Fast deterministic state cloning
2. Rollout-policy interface
3. Leaf evaluator interface
4. Root-level flat Monte Carlo
5. Tournament validation
6. Learned evaluators
7. Progressive widening
8. Search tree
9. MCTS
10. Search-generated training targets
11. Opponent modeling
12. Production optimization

## Core Principles

- Engine remains authoritative.
- Search never invents illegal moves.
- Search budgets are explicit.
- Rollouts are reproducible.
- Evaluate strength by tournaments, not offline metrics.
- Improve evaluator and search together.

## Suggested Reading Order

1. UCT paper
2. AlphaGo paper
3. Risk drafting paper
4. Flat Monte Carlo implementation
5. Learned evaluator integration
6. MCTS
7. Search distillation
