
# Global Conquest – GCN Strategy Roadmap

> This roadmap describes a practical path toward building a Graph Convolutional Network (GCN) based Risk bot around your existing Go engine, simulator, and tournament framework.

## Primary References

### Risk-specific

1. **Using Graph Convolutional Networks and TD(λ) to Play the Game of Risk** (Jamie Carr, 2020)
   - https://arxiv.org/abs/2009.06355
   - The single most relevant paper. Introduces a GCN value network trained with TD(λ) and combined with search.

### Supporting Graph Learning

2. **Semi-Supervised Classification with Graph Convolutional Networks**
   - https://arxiv.org/abs/1609.02907
   - Original GCN paper.

3. **PyTorch Geometric Documentation**
   - https://pytorch-geometric.readthedocs.io/
   - Practical implementation reference.

## Recommended Architecture

```text
Game State
    ↓
Generate Legal Candidates
    ↓
Construct Candidate Afterstates
    ↓
Encode Graph
    ↓
GCN Value Network
    ↓
Choose Highest-Valued Candidate
```

## Major Milestones

1. Versioned graph-state schema
2. Player-relative node encoding
3. Candidate-afterstate generation
4. Candidate-grouped datasets
5. Baseline models (LogReg / GBT / MLP)
6. Supervised GCN
7. Shadow mode
8. Hybrid heuristic + GCN strategy
9. Tournament validation
10. TD learning
11. Search integration
12. Multi-map generalization

## Key Design Principles

- Keep rules in Go.
- Train in Python.
- Evaluate afterstates, not current states.
- Promote only via tournament results.
- Version graph schemas.
- Split datasets by game, never by position.
- Treat the GCN as a value network before considering a policy network.

## Suggested Reading Order

1. Carr Risk paper
2. Original GCN paper
3. PyTorch Geometric docs
4. Implement graph schema
5. Build datasets
6. Train supervised evaluator
7. Introduce TD learning
8. Integrate search
