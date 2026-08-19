---
title: "React Mental Models — How to Actually Think in React"
summary: >
  The mental model of React as it was designed: UI as a projection of data into a tree,
  rendering as a pure function, state as a snapshot, effects as synchronization with external
  systems, component identity as position-in-tree, and the "two computers" framing of RSC.
  Assembled from Dan Abramov's essays (overreacted.io), the React docs (react.dev), and
  Sebastian Markbåge's foundational gists and talks.
source_count: 30
---

# React Mental Models — How to Actually Think in React

> "The core premise for React is that UIs are simply a projection of data into a different form of data." — Sebastian Markbåge, *react-basic*

> "Idempotence is more important to React than purity." — Dan Abramov, *React as a UI Runtime*

---

## 1. UI as a tree, components as pure functions

React programs output a **tree that may change over time**. The "host tree" — DOM, native views — is what React manipulates; React itself is a layer on top.

> "React programs usually output a tree that may change over time." — Abramov, *React as a UI Runtime*

> "The host tree is relatively stable and most updates don't radically change its overall structure." — Abramov

A **React element** is a plain JS object describing a host instance. Elements are disposable — re-created and thrown away every render. Think of them as frames in a movie.

> "A React element is a plain JavaScript object. It can describe a host instance." — Abramov

> "I like to think of React elements as being like frames in a movie. They capture what the UI should look like at a specific point in time." — Abramov

**Components are pure functions of their inputs.** Same props → same output. They don't coordinate during rendering; they don't mutate objects that existed before rendering; they mind their own business.

> "React assumes that every component you write is a pure function. This means that React components you write must always return the same JSX given the same inputs." — react.dev, *Keeping Components Pure*

> "Each component should only 'think for itself', and not attempt to coordinate with or depend upon others during rendering." — react.dev

> "The same input gives the same output. A simple pure function." — Markbåge, *react-basic*

Purity unlocks the ability to run components in other environments — notably the server.

> "Writing pure functions takes some habit and discipline. But it also unlocks marvelous opportunities: Your components could run in a different environment—for example, on the server!" — react.dev

**Local mutation is fine.** Mutating something you just created inside render is allowed.

> "It's completely fine to change variables and objects that you've just created while rendering. This is called 'local mutation'—it's like your component's little secret." — react.dev

**Purity in React really means idempotence.**

> "Pure in general has a specific meaning but can vary by definition. In React, it mostly means that it has to be idempotent. It will always return the same thing for the same input." — Markbåge, *The Rules of React*

---

## 2. Render and commit — what "rendering" actually means

The update cycle has three phases:

1. **Trigger** — something asks for a render (initial mount or state update).
2. **Render** — React calls your component. That's it.
3. **Commit** — React applies the minimal DOM diff.

> "Imagine that your components are cooks in the kitchen, assembling tasty dishes from ingredients. In this scenario, React is the waiter who puts in requests from customers and brings them their orders." — react.dev, *Render and Commit*

> "After you trigger a render, React calls your components to figure out what to display on screen. 'Rendering' is React calling your components." — react.dev

> "React only changes the DOM nodes if there's a difference between renders." — react.dev

A re-render is not a bug. It's the calculation React does to figure out what to commit. Optimizing "to avoid re-renders" before measurement is the wrong mental model.

> "The process of figuring out what to do to the host instance tree in response to new information is sometimes called reconciliation." — Abramov

---

## 3. State as a snapshot

The single most important mental model in React. Setting state does **not** mutate a variable — it schedules a re-render. Inside the current render, the state variable stays exactly what it was when the component was called.

> "State variables might look like regular JavaScript variables that you can read and write to. However, state behaves more like a snapshot. Setting it does not change the state variable you already have, but instead triggers a re-render." — react.dev, *State as a Snapshot*

> "Setting state only changes it for the next render. During the first render, `number` was `0`. This is why, in that render's `onClick` handler, the value of `number` is still `0` even after `setNumber(number + 1)` was called." — react.dev

The canonical example:

```jsx
// number is 0
<button onClick={() => {
  setNumber(number + 1);
  setNumber(number + 1);
  setNumber(number + 1);
}}>
```

> "Since the `number` state variable is `0` for this render, its event handler looks like this: `setNumber(0 + 1); setNumber(0 + 1); setNumber(0 + 1);`" — react.dev

Result: `1`, not `3`. For three increments, use the updater form.

**Debugging heuristic:** "You can mentally substitute state variables with their values in your code." — react.dev

**State lives on a shelf.** React owns it; your component function is re-called with a fresh snapshot.

> "React stores state outside of your component, as if on a shelf." — react.dev

Closures make this rigorous:

> "Function components capture the rendered values." — Abramov, *How Are Function Components Different from Classes?*

> "Our event handlers 'belong' to a particular render with particular props and state." — Abramov

> "Inside any particular render, props and state forever stay the same." — Abramov

> "Every function inside the component render (including event handlers, effects, timeouts or API calls inside them) captures the props and state of the render call that defined it." — Abramov

---

## 4. Batching and updater functions

React waits until all event handler code runs before processing state updates.

> "React waits until all code in the event handlers has run before processing your state updates." — react.dev, *Queueing a Series of State Updates*

> "React does not batch across multiple intentional events like clicks—each click is handled separately." — react.dev

For multiple updates to the same state before the next render, use an **updater function**:

```jsx
setNumber(n => n + 1);
setNumber(n => n + 1);
setNumber(n => n + 1);
// result: +3
```

> "When you pass it to a state setter: 1. React queues this function to be processed after all the other code in the event handler has run. 2. During the next render, React goes through the queue and gives you the final updated state." — react.dev

> "Updater functions run during rendering, so updater functions must be pure and only return the result." — react.dev

---

## 5. Effects are synchronization, not lifecycle

The largest source of bad React code is thinking of `useEffect` as `componentDidMount`. It isn't. Effects are an **escape hatch** to synchronize with systems outside of React.

> "Effects are an escape hatch from the React paradigm. They let you 'step outside' of React and synchronize your components with some external system like a non-React widget, network, or the browser DOM." — react.dev, *You Might Not Need an Effect*

> "If there is no external system involved (for example, if you want to update a component's state when some props or state change), you shouldn't need an Effect." — react.dev

The diagnostic question: *why does this code need to run?*

> "Use Effects only for code that should run because the component was displayed to the user." — react.dev

> "Effects let you specify side effects that are caused by rendering itself, rather than by a particular event. Sending a message in the chat is an event because it is directly caused by the user clicking a specific button. However, setting up a server connection is an Effect because it should happen no matter which interaction caused the component to appear." — react.dev, *Synchronizing with Effects*

Effects start and stop synchronizing. They do not mount/update/unmount.

> "Effects have a different lifecycle from components. Components may mount, update, or unmount. An Effect can only do two things: to start synchronizing something, and later to stop synchronizing it." — react.dev, *Lifecycle of Reactive Effects*

> "Instead, always focus on a single start/stop cycle at a time. It shouldn't matter whether a component is mounting, updating, or unmounting." — react.dev

> "The right question isn't 'how to run an Effect once', but 'how to fix my Effect so that it works after remounting'." — react.dev

### Reactivity

Every value declared inside the component body is **reactive** — effects must react to it.

> "Props, state, and other values declared inside the component are reactive because they're calculated during rendering and participate in the React data flow." — react.dev

> "In other words, Effects 'react' to all values from the component body." — react.dev

> "Logic inside event handlers is not reactive. It will not run again unless the user performs the same interaction." — react.dev, *Separating Events from Effects*

### Dependencies are a truth-claim, not a preference

> "Notice that you can't 'choose' your dependencies. You will get a lint error if the dependencies you specified don't match what React expects based on the code inside your Effect." — react.dev

> "Dependencies are our hint to React about everything that the effect uses from the render scope." — Abramov, *A Complete Guide to useEffect*

> "Lying to React about dependencies has bad consequences." — Abramov

### Abramov's unlock

> "It's only after I stopped looking at the `useEffect` Hook through the prism of the familiar class lifecycle methods that everything came together for me." — Abramov

> "Conceptually, you can imagine effects are a part of the render result." — Abramov

> "`useEffect` lets you synchronize things outside of the React tree according to our props and state." — Abramov

> "It's not the `count` variable that somehow changes inside an 'unchanging' effect. It's the effect function itself that's different on every render." — Abramov

---

## 6. Component identity is position-in-tree

React identifies components by their **position in the rendered tree**, not by JSX source location or variable name.

> "State is isolated between components. React keeps track of which state belongs to which component based on their place in the UI tree." — react.dev, *Preserving and Resetting State*

> "When you give a component state, you might think the state 'lives' inside the component. But the state is actually held inside React." — react.dev

**The canonical rule:**

> "React preserves a component's state for as long as it's being rendered at its position in the UI tree. If it gets removed, or a different component gets rendered at the same position, React discards its state." — react.dev

> "It's the same component at the same position, so from React's perspective, it's the same counter." — react.dev

> "Remember that it's the position in the UI tree—not in the JSX markup—that matters to React!" — react.dev

> "When you render a different component in the same position, it resets the state of its entire subtree." — react.dev

**Consequence:** never nest component definitions. You'll get a brand-new component type every render.

> "Always declare component functions at the top level, and don't nest their definitions." — react.dev

---

## 7. Keys — position override, not global ID

Keys override the sibling-order identity with something you choose.

> "Specifying a `key` tells React to use the `key` itself as part of the position, instead of their order within the parent." — react.dev

> "Remember that keys are not globally unique. They only specify the position within the parent." — react.dev

> "A well-chosen `key` provides more information than the position within the array. Even if the position changes due to reordering, the `key` lets React identify the item throughout its lifetime." — react.dev, *Rendering Lists*

> "Imagine that files on your desktop didn't have names. Instead, you'd refer to them by their order — the first file, the second file, and so on. You could get used to it, but once you delete a file, it would get confusing." — react.dev

> "Keys must not change or that defeats their purpose! Don't generate them while rendering." — react.dev

**Resetting state with a key is idiomatic React.** Don't write an effect that calls `setState(initial)` when a prop changes. Key the component by that prop.

> "By passing `userId` as a `key` to the `Profile` component, you're asking React to treat two `Profile` components with different `userId` as two different components that should not share any state." — react.dev

Index-as-key is almost always wrong:

> "You might be tempted to use an item's index in the array as its key. But the order in which you render items will change over time if an item is inserted, deleted, or if the array gets reordered. Index as a key often leads to subtle and confusing bugs." — react.dev

---

## 8. Lifting, controlled/uncontrolled, derived state

**Lifting state up** is the canonical pattern for two components that change together.

> "Sometimes, you want the state of two components to always change together. To do it, remove state from both of them, move it to their closest common parent, and then pass it down to them via props." — react.dev, *Sharing State Between Components*

Controlled vs uncontrolled is a spectrum:

> "It is common to call a component with some local state 'uncontrolled'. In contrast, you might say a component is 'controlled' when the important information in it is driven by props rather than its own local state." — react.dev

> "For each unique piece of state, you will choose the component that 'owns' it. This principle is also known as having a 'single source of truth'." — react.dev

**Avoid redundant state.** Derive during render.

> "Avoid redundant state. If you can calculate some information from the component's props or its existing state variables during rendering, you should not put that information into that component's state." — react.dev, *Choosing the State Structure*

> "Make your state as simple as it can be—but no simpler." — react.dev

**Do not copy props into state.**

> "Don't stop the data flow." — Abramov, *Writing Resilient Components*

> "By copying a prop into state you're ignoring all updates to it." — Abramov

> "Read the props directly in your component and avoid copying props into state." — Abramov

### Declarative vs imperative

> "In React, you don't directly manipulate the UI—meaning you don't enable, disable, show, or hide components directly. Instead, you declare what you want to show, and React figures out how to update the UI." — react.dev, *Reacting to Input with State*

> "Think of getting into a taxi and telling the driver where you want to go instead of telling them exactly where to turn." — react.dev

---

## 9. Server components — the two-computer mental model

Abramov's RSC framing: showing something on your screen involves two computers. Code has to run *somewhere*.

> "Components are code, and that code has to run somewhere. But wait—whose computer should they run on?" — Abramov, *The Two Reacts*

**Client React** is `UI = f(state)`. State lives on your machine, so the code has to run there too.

**Server React** is `UI = f(data)`. Data lives on the server; components there can read it directly.

> "There is nothing extra I need to do because my code runs right where the data is." — Abramov

> "Running my components close to their data source lets them read their own data and preprocess it before sending any of that information to your device." — Abramov

The full picture: `UI = f(data, state)` — split across two programming environments.

### Early world / Late world

> "A tag is a potential function call. A tag is a proto-call." — Abramov, *React for Two Computers*

> "The data flows strictly in a one direction—from the first to the second computer." — Abramov

> "Components are the 'brains' of our program—they figure out what needs to be done. Primitives are the 'muscles'—they actually do stuff after most of the thinking has already been done." — Abramov

> "Think before you do." — Abramov

### The inversion

> "Your components don't call your API. Instead, your API returns your components." — Abramov, *JSX Over The Wire*

> "On the server, it's ViewModels all the way down." — Abramov

---

## 10. React team principles

> "We don't start with the abstraction itself. Instead, we start with the desired user experience, and work backwards to the abstraction." — Abramov, *What Are the React Team Principles?*

> "Making React internals simple is not a goal. We are willing to make React internals complex if that complexity lets product developers keep their code easier to understand and modify." — Abramov

> "We provide escape hatches so people can work around us where necessary." — Abramov

> "When designing APIs, we assume the person only has local knowledge about the piece of code they're working on." — Abramov

Markbåge on API design:

> "It's much easier to recover from no abstraction than the wrong abstraction." — Markbåge, *Minimal API Surface Area*

> "Explicit -> Implicit is easy. Implicit -> Explicit is hard." — Markbåge

---

## Sources

1. https://overreacted.io/react-as-a-ui-runtime/
2. https://overreacted.io/a-complete-guide-to-useeffect/
3. https://overreacted.io/how-are-function-components-different-from-classes/
4. https://overreacted.io/before-you-memo/
5. https://overreacted.io/writing-resilient-components/
6. https://overreacted.io/the-two-reacts/
7. https://overreacted.io/react-for-two-computers/
8. https://overreacted.io/jsx-over-the-wire/
9. https://overreacted.io/the-elements-of-ui-engineering/
10. https://overreacted.io/what-are-the-react-team-principles/
11. https://react.dev/learn/thinking-in-react
12. https://react.dev/learn/keeping-components-pure
13. https://react.dev/learn/render-and-commit
14. https://react.dev/learn/state-as-a-snapshot
15. https://react.dev/learn/queueing-a-series-of-state-updates
16. https://react.dev/learn/reacting-to-input-with-state
17. https://react.dev/learn/choosing-the-state-structure
18. https://react.dev/learn/sharing-state-between-components
19. https://react.dev/learn/preserving-and-resetting-state
20. https://react.dev/learn/rendering-lists
21. https://react.dev/learn/escape-hatches
22. https://react.dev/learn/synchronizing-with-effects
23. https://react.dev/learn/lifecycle-of-reactive-effects
24. https://react.dev/learn/separating-events-from-effects
25. https://react.dev/learn/you-might-not-need-an-effect
26. https://github.com/reactjs/react-basic/blob/master/README.md
27. https://gist.github.com/sebmarkbage/75f0838967cd003cd7f9ab938eb1958f
28. https://2014.jsconf.eu/speakers/sebastian-markbage-minimal-api-surface-area-learning-patterns-instead-of-frameworks.html
29. https://www.youtube.com/watch?v=4anAwXYqLG8
30. https://react.dev/
