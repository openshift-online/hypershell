# Deterministic production stories

Never call a live session, BFF, SDK, API, metrics endpoint, or deployed service
from a Storybook parity story. Mocking runtime boundaries in Storybook does not
change production behavior and requires no additional user authorization.

The inverse rule also applies: never place Storybook scenarios, mock records,
sample counts, or fixture selectors in runtime production modules.

## Required production boundary

Split the implementation into these roles:

- **Route/container:** call real application hooks, loaders, and composition
  adapters. Convert their results into view props.
- **Production view:** render typed data and callbacks received through props.
  Do not fetch and do not select named fixture scenarios.
- **Story:** import the production view and translate named Storybook scenarios
  into complete deterministic view props or injected in-memory adapters.

Scenario names such as `standard`, `admin`, `partial`, or `cluster-down` may
index Storybook/mockup fixtures. They must not be props accepted by production
views or values passed by runtime routes. Production props describe facts such
as `canProvision`, gateway records, cluster health, metrics, loading state, and
errors.

Before implementation, record this table for every dynamic value shown:

| Visible data | Runtime source | Production prop | Story fixture |
|---|---|---|---|

Every runtime source must be a real application port, composition adapter,
loader, or query. Missing APIs must render an explicit unavailable state; do
not substitute plausible fixture values.

## Selection order

Use the first applicable pattern:

1. Extract a production view component that accepts serializable state and
   callbacks as props. Keep hooks and live adapters in the route/container.
   Render that same production view component from the story with fixtures.
2. Pass production service interfaces through the existing provider and supply
   deterministic in-memory adapters in the story.
3. For TanStack Query, create a fresh `QueryClient` per story, disable retries
   and refetches, and provide deterministic query functions or seeded cache for
   every query the component reaches.
4. For React Router loaders/actions, create a memory router with deterministic
   loader/action implementations. `MemoryRouter` alone does not mock data.

Do not duplicate page markup in the story. Do not modify the production
composition root to use mock data outside Storybook.

Run the boundary checker before capture:

```bash
python3 skills/build/ui-build-gate/scripts/check_production_data_boundary.py \
  --route <runtime-route.tsx> \
  --view <production-view.tsx> \
  --story <production-story.stories.tsx> \
  --required-source <real-hook-or-adapter> [--required-source ...]
```

Pass one `--required-source` for every runtime source in the data table.

## HyperShell seams

- `useSession()` calls `sessionGateway.getSession()`. Supply a deterministic
  `BrowserSession` through a view prop, injected session adapter, or isolated
  query harness.
- `gatewayOperations` is a live composition adapter. Supply a deterministic
  gateway operations implementation or pass its resolved records into a
  production view component.
- Use a new `QueryClient` for each story/state. The global preview client can
  retain data across stories and must not define state-specific fixtures.
- Reuse fixture factories exported by canonical UI packages when available.

Expose one story for every materially different state required by the mockup:
permission, administrator visibility, healthy/attention, partial/unavailable,
empty, loading, and error states as applicable. Give corresponding production
and mockup states the same names.

Before capture, verify the story renders its final state without a live server.
If a dependency is difficult to mock, refactor the production boundary; do not
stop or hand off.
