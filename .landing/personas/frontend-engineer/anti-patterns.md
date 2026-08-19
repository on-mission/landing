---
title: "React Anti-patterns — What Not to Do, and What to Do Instead"
summary: >
  The sharpest file in the corpus. Catalog of common React mistakes — useEffect overreach,
  premature memoization, premature abstraction, Context abuse, global state for everything,
  testing implementation details, index-as-key, missing cleanup, derived state, clean-code
  cargo cult — with verbatim "don't do this, do this" guidance from react.dev, Dan Abramov,
  Kent C. Dodds, Mark Erikson, and Sandi Metz.
source_count: 20
---

# React Anti-patterns — What Not to Do, and What to Do Instead

> "If you're not trying to synchronize with some external system, you probably don't need an Effect." — react.dev

> "Setting state immediately in an Effect is like plugging a power outlet into itself." — react.dev

## 1. useEffect overreach

**The single biggest source of bad React code.** If you're using `useEffect` to mirror state, resolve derived values, notify the parent, or chain actions — stop.

### Do instead
- **Calculate during render.** If it can be derived from props/state, derive it.
- **Put event logic in event handlers.** Code that runs because of a user action belongs there.
- **Use `key` to reset state.** `<Child key={propId} />`.
- **Update both components in the same event handler.** To notify parents, call both setters in the originating event.
- **Lift fetching up** or use a framework data layer / React Query / SWR.
- **Use `useSyncExternalStore`.** Don't hand-roll store subscriptions with effects.
- **Use `useMemo`** only for *measurably* expensive derivations.

### Quotes

> "If you're not trying to synchronize with some external system, you probably don't need an Effect." — react.dev

> "You don't need Effects to transform data for rendering." — react.dev, *You Might Not Need an Effect*

> "You don't need Effects to handle user events." — react.dev

> "Code that runs because a component was displayed should be in Effects, the rest should be in events." — react.dev

> "When something can be calculated from the existing props or state, don't put it in state." — react.dev

> "🔴 Avoid: redundant state and unnecessary Effect... This is more complicated than necessary. It is inefficient too." — react.dev

> "🔴 Avoid: Resetting state on prop change in an Effect... This is inefficient because `ProfilePage` and its children will first render with the stale value, and then render again." — react.dev

> "🔴 Avoid: Event-specific logic inside an Effect... This Effect is unnecessary. It will also most likely cause bugs." — react.dev

> "🔴 Avoid: Chains of Effects that adjust the state solely to trigger each other... Such code is often rigid and fragile." — react.dev

> "🔴 Avoid: Passing data to the parent in an Effect. When child components update the state of their parent components in Effects, the data flow becomes very difficult to trace." — react.dev

> "Setting state immediately in an Effect is like plugging a power outlet into itself." — react.dev

### The 12 cases from *You Might Not Need an Effect*

1. **Updating state based on props or state** → compute during render.
2. **Caching expensive calculations** → `useMemo`, only if measurably expensive.
3. **Resetting all state when a prop changes** → `<Profile key={userId} />`.
4. **Adjusting some state when a prop changes** → adjust during render with a prev-sentinel, or store IDs.
5. **Sharing logic between event handlers** → extract a function; call from both handlers.
6. **Sending a POST request** → fire in the submit handler.
7. **Chains of computations** → compute in the event handler.
8. **Initializing the application** → module-level or `if (typeof window !== 'undefined')`.
9. **Notifying parent components about state changes** → call `onChange` in the same handler that calls `setState`.
10. **Passing data to the parent** → move the fetch/ownership to the parent.
11. **Subscribing to an external store** → `useSyncExternalStore`.
12. **Fetching data** → framework loader, React Query, SWR, or at least a proper cleanup guard.

### Abramov's unlock

> "It's only after I stopped looking at the `useEffect` Hook through the prism of the familiar class lifecycle methods that everything came together for me." — Abramov

---

## 2. Premature memoization

`useMemo` and `useCallback` everywhere by default make code more complex, add dependency surface area, and often don't help.

### Do instead
- **Split components** so the parts that change are separate from the parts that don't.
- **Move state down** into the smallest component that needs it.
- **Lift content up as `children`** so a re-rendering parent doesn't re-render the subtree it passes through.

### Quotes

> "Before you apply optimizations like `memo` or `useMemo`, it might make sense to look if you can split the parts that change from the parts that don't change." — Abramov, *Before You memo()*

> "When the `color` changes, `ColorPicker` re-renders. But it still has the same `children` prop it got from the `App` last time, so React doesn't visit that subtree." — Abramov

> "Using the `children` prop to split up components usually makes the data flow of your application easier to follow and reduces the number of props plumbed down through the tree. Improved performance in cases like this is a cherry on top, not the end goal." — Abramov

> "If removing an optimization breaks a component, it was too fragile to begin with." — Abramov

### Named anti-patterns
- **`useMemo`/`useCallback` on every value** — the dependency array becomes its own source of bugs.
- **`React.memo` on components whose props change every render** — shallow equality always fails; you pay the compare cost for nothing.
- **Reaching for memoization before restructuring.**

---

## 3. Over-abstracting / premature hooks extraction

### Do instead
- **Prefer duplication until the right shape emerges.**
- **Inline code back** to call sites when the abstraction is wrong.
- **Rule of Three.** Wait for the third occurrence before extracting.

### Quotes

> "duplication is far cheaper than the wrong abstraction" — Sandi Metz

> "prefer duplication over the wrong abstraction" — Sandi Metz

> "If you find yourself passing parameters and adding conditional paths through shared code, the abstraction is incorrect." — Sandi Metz

> "When dealing with the wrong abstraction, the fastest way forward is back." — Sandi Metz

> "Re-introduce duplication by inlining the abstracted code back into every caller." — Sandi Metz

> "Avoid Hasty Abstractions." — Kent C. Dodds

> "I'm fine with code duplication until you feel pretty confident that you know the use cases for that duplicate code." — Kent C. Dodds

> "Obsessing with 'clean code' and removing duplication is a phase many of us go through." — Dan Abramov

> "My code traded the ability to change requirements for reduced duplication, and it was not a good trade." — Abramov

> "Let clean code guide you. Then let it go." — Abramov

> "Rewriting your teammate's code without a discussion is a huge blow to your ability to effectively collaborate." — Abramov

### Named anti-patterns
- **The generic hook that grows options.** Every new caller adds a boolean flag.
- **Extracting a custom hook used exactly once.** DRY of one.
- **Shared code that carves control flow through conditionals for every caller.** Inline it.

---

## 4. Prop-drilling paranoia and Context abuse

### Do instead
- **Use component composition first.** Pass JSX as `children` or slot props.
- **Keep context scoped.** Multiple small contexts, placed deep in the tree where actually needed.
- **Don't reach for Context to avoid three levels of props.**

### Quotes

> "Context is a form of Dependency Injection. It is a transport mechanism - it doesn't 'manage' anything." — Mark Erikson

> "All components subscribed to that context will be forced to re-render, even if they only care about part of the data." — Erikson

> "If you get past 2-3 state-related contexts in an application, you're re-inventing a weaker version of React-Redux and should just switch to using Redux." — Erikson

> "At that point you're just reinventing React-Redux, poorly." — Erikson

> "you shouldn't be reaching for context to solve every state sharing problem that crosses your desk" — Kent C. Dodds

> "There's no reason to break things out prematurely. Wait until you really need to reuse a block before breaking it out." — Kent C. Dodds

> "Extract components and pass JSX as children to them. If you pass some data through many layers of intermediate components that don't use that data (and only pass it further down), this often means that you forgot to extract some components along the way." — React docs

### Named anti-patterns
- **Single "AppContext" holding everything.** Forces the whole tree to re-render on any change.
- **Context + useReducer as Redux-substitute without subscription granularity.**
- **Reaching for Context to skip two levels of props.**

---

## 5. Global state for everything

### Do instead
- **Co-locate state** — put it in the component that uses it.
- **Use React Query / SWR for server cache** — not Redux, not Zustand.
- **Lift only as far as necessary. No further.**

### Quotes

> "don't use Redux until you have problems with vanilla React." — Dan Abramov

> "You'll know when you need Flux. If you aren't sure if you need it, you don't need it." — Pete Hunt

> "Don't use Redux unless you're unhappy with local component state." — Abramov

> "If your reducer looks boring, don't use redux." — Abramov

> "Not all apps need Redux." — Redux FAQ

> "If the only thing you needed to do with Redux is avoid passing data as props through 15 levels of your components — well, that's literally what Context was invented to do." — Mark Erikson

> "I consistently see developers putting all of their state into redux. Not just global application state, but local state as well... for simple state (like whether a modal is open or form input value state) this is a big problem." — Kent C. Dodds

> "Server cache has inherently different problems from UI state and therefore needs to be managed differently." — Kent C. Dodds

> "If you embrace the fact that what you have is not actually state at all but is instead a cache of state, then you can start thinking about it correctly and therefore managing it correctly." — Kent C. Dodds

> "One of the leading causes to slow React applications is global state, especially the rapidly changing variety." — Kent C. Dodds

> "When we manage the state higher up in the React component tree, every update to that state results in an invalidation of the entire React tree." — Kent C. Dodds

> "Avoid making truly local state global." — Dan Abramov

### Named anti-patterns
- **Server data dumped in Redux/Zustand.** Reinvents cache invalidation poorly.
- **Form values in global store.** Belongs in the form component.
- **Modal open/close in global store.** Belongs in the modal's parent.
- **"Lift state up to the root" as default instinct.** Whole-tree re-renders.

---

## 6. Testing implementation details

### Do instead
- **Test what the user sees and does.** Query by role/text; interact with `user-event`.
- **Use `@testing-library/react` and `@testing-library/user-event`.** Avoid Enzyme shallow rendering.
- **Only use `query*` for asserting absence.**

### Quotes

> "The more your tests resemble the way your software is used, the more confidence they can give you." — Kent C. Dodds

> "Implementation details are things which users of your code will not typically use, see, or even know about." — Kent C. Dodds

> "Tests which test implementation details can give you a false negative when you refactor your code." — Kent C. Dodds

> "I completely avoided shallow rendering, never used APIs like `instance()`, `state()`, or `find('ComponentName')`." — Kent C. Dodds

> "The only reason the `query*` variant is exposed is to verify that an element is not rendered." — Kent C. Dodds

> "`fireEvent.change` will simply trigger a single change event... `type` will trigger `keyDown`, `keyPress`, and `keyUp` events for each character as well." — Kent C. Dodds

### Kent's 18-item common-mistakes list (from *Common Mistakes with React Testing Library*)
1. Not using the ESLint plugins.
2. Using `wrapper` as the destructuring variable name.
3. Manually calling `cleanup`.
4. Not using `screen` for queries.
5. Using generic assertions instead of `jest-dom` matchers.
6. Wrapping things in `act()` unnecessarily.
7. Using the wrong query (reaching for `getByTestId` when `getByRole` would do).
8. Querying via `container.querySelector`.
9. Not querying by visible text.
10. Not using `*ByRole` most of the time.
11. Adding `aria-*`/`role` attributes willy nilly.
12. Using `fireEvent` where `userEvent` would better simulate real interaction.
13. Misusing `query*` variants.
14. Using `waitFor` where `findBy` is simpler.
15. Passing an empty callback to `waitFor`.
16. Multiple assertions in a single `waitFor` callback.
17. Performing side effects inside `waitFor`.
18. Using `get*` variants as redundant assertions.

---

## 7. Index as key

### Do instead
- **Use a stable ID from the data.**
- **Never `key={Math.random()}`** and never generate keys during render.

### Quotes

> "You might be tempted to use an item's index in the array as its key... But the order in which you render items will change over time if an item is inserted, deleted, or if the array gets reordered. Index as a key often leads to subtle and confusing bugs." — react.dev

> "Similarly, do not generate keys on the fly, e.g. with `key={Math.random()}`. This will cause keys to never match up between renders, leading to all your components and DOM being recreated every time. Not only is this slow, but it will also lose any user input inside the list items. Instead, use a stable ID based on the data." — react.dev

> "Keys must be unique among siblings... Keys must not change or that defeats their purpose! Don't generate them while rendering." — react.dev

### Named anti-patterns
- **`key={index}`** — breaks reconciliation on insertion/removal/reorder; lost input state; wrong DOM reuse.
- **`key={Math.random()}`** — full remount every render.
- **Key generated in render (`key={uuid()}`)** — same as random.

---

## 8. useEffect without cleanup / race conditions

### Do instead
- **Every subscription needs a symmetric teardown.** StrictMode's double-mount reveals missing cleanup.
- **Guard async in effects with `let ignore = false`.**
- **Don't use refs to suppress double-firing in development.**

### Quotes

> "Remounting components only happens in development to help you find Effects that need cleanup." — react.dev

> "A common pitfall for preventing Effects firing twice in development is to use a `ref` to prevent the Effect from running more than once... This won't fix the bug!" — react.dev

> "To fix the bug, it is not enough to just make the Effect run once. The effect needs to work after re-mounting, which means the connection needs to be cleaned up." — react.dev

> "Bugs like this are called race conditions because two asynchronous operations are 'racing' with each other, and they might arrive in an unexpected order." — react.dev

> "You can't 'undo' a network request that already happened, but your cleanup function should ensure that the fetch that's not relevant anymore does not keep affecting your application." — react.dev

> "When dependencies don't match the code, there is a high risk of introducing bugs. By suppressing the linter, you 'lie' to React about the values your Effect depends on." — react.dev

> "Lying to React about dependencies has bad consequences." — Dan Abramov

> "Each render has its own Props and State... Inside any particular render, props and state forever stay the same." — Abramov

### Named anti-patterns
- **Subscriptions without matching cleanup.**
- **Data fetching without a stale-response guard.**
- **Silencing `react-hooks/exhaustive-deps`.**
- **Using a ref to make an effect "only run once"** — papers over missing cleanup.

---

## 9. Controlled components done wrong / derived state

### Do instead
- **Control fully or leave uncontrolled — don't straddle.**
- **Don't copy props into state.** If the parent owns the value, read it from props.
- **Use `key` to reset — not a sync effect.**

### Quotes

> "By copying a prop into state you're ignoring all updates to it." — Dan Abramov

> "Props and state are a part of the React data flow. Both rendering and side effects should reflect changes in that data flow, not ignore them!" — Abramov

> "The `getDerivedStateFromProps` method is clunky." — Abramov

> "It entirely relies on accidental timing." — Abramov

> "If its parent re-renders more often, it will keep blowing away the child state!" — Abramov

> "Components should be resilient to rendering less or more often because otherwise they're too coupled to their particular parents." — Abramov

### Named anti-patterns
- **`value` prop with no `onChange`** — React warns; input is effectively read-only.
- **Copying `props.value` into state** and trying to "keep in sync."
- **Using `getDerivedStateFromProps`** to mirror props into state.
- **A `useEffect` that calls `setState` whenever a prop changes** — just read the prop.

---

## 10. "Clean code" cargo cult

### Do instead
- **Prefer duplication** until the abstraction becomes obvious.
- **Talk to the author** before rewriting their code for "cleanliness."
- **Optimize for change, not for elegance.**

### Quotes

> "prefer duplication over the wrong abstraction" — Sandi Metz

> "Obsessing with 'clean code' and removing duplication is a phase many of us go through." — Dan Abramov

> "Clean code is not a goal. It's an attempt to make some sense out of immense complexity." — Abramov

> "Let clean code guide you. Then let it go." — Abramov

> "I didn't talk to the person who wrote it. I rewrote the code and checked it in without their input." — Abramov

> "Rewriting your teammate's code without a discussion is a huge blow to your ability to effectively collaborate." — Abramov

### Named anti-patterns
- **"Clean code" rewrites of colleagues' work without a conversation.**
- **Hunting duplication as a reflex.**
- **Identity-driven refactoring** ("I'm the kind of engineer who writes clean code").

---

## 11. The four Resilient Components principles

From Abramov's *Writing Resilient Components*:

1. **Don't stop the data flow.**
2. **Always be ready to render.**
3. **No component is a singleton.**
4. **Keep the local state isolated.**

Most anti-patterns in this file violate one of these four.

---

## Sources

1. https://react.dev/learn/you-might-not-need-an-effect
2. https://react.dev/reference/react/useEffect
3. https://react.dev/learn/synchronizing-with-effects
4. https://react.dev/learn/rendering-lists
5. https://react.dev/learn/sharing-state-between-components
6. https://overreacted.io/a-complete-guide-to-useeffect/
7. https://overreacted.io/before-you-memo/
8. https://overreacted.io/writing-resilient-components/
9. https://overreacted.io/goodbye-clean-code/
10. https://kentcdodds.com/blog/testing-implementation-details
11. https://kentcdodds.com/blog/common-mistakes-with-react-testing-library
12. https://kentcdodds.com/blog/how-to-use-react-context-effectively
13. https://kentcdodds.com/blog/aha-programming
14. https://kentcdodds.com/blog/application-state-management-with-react
15. https://kentcdodds.com/blog/state-colocation-will-make-your-react-app-faster
16. https://kentcdodds.com/blog/prop-drilling
17. https://sandimetz.com/blog/2016/1/20/the-wrong-abstraction
18. https://blog.isquaredsoftware.com/2021/01/context-redux-differences/
19. https://changelog.com/posts/when-and-when-not-to-reach-for-redux
20. https://redux.js.org/faq/general
