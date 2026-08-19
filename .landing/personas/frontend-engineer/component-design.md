---
title: "Component Design — Composition, Custom Hooks, and the Discipline of Abstraction"
summary: >
  How to structure React components: composition over inheritance, custom hooks as the primary
  abstraction, compound components, render props, state reducers, and the discipline to NOT
  abstract until the shape is clear. Synthesized from Kent C. Dodds' pattern work, Dan Abramov's
  "Writing Resilient Components" and his 2019 retraction of Presentational/Container, Sandi Metz's
  "The Wrong Abstraction", and the React docs on custom hooks and Thinking in React.
source_count: 22
---

# Component Design — Composition, Custom Hooks, and the Discipline of Abstraction

> "Make your abstraction do less stuff, and make your users do that instead." — Kent C. Dodds, *Inversion of Control*

> "duplication is far cheaper than the wrong abstraction" — Sandi Metz, *The Wrong Abstraction*

---

## 1. Composition over inheritance

React's entire component model is built on composition. There is no inheritance. You get reuse through `children`, slot props, and specialization by configuration.

> "React has a powerful composition model, and we recommend using composition instead of inheritance to reuse code between components." — React legacy docs

**Containment** — components that don't know their children ahead of time:

> "Some components don't know their children ahead of time. This is especially common for components like Sidebar or Dialog that represent generic 'boxes'. We recommend that such components use the special children prop to pass children elements directly into their output." — React legacy docs

```jsx
function FancyBorder(props) {
  return (
    <div className={'FancyBorder FancyBorder-' + props.color}>
      {props.children}
    </div>
  );
}
```

**Slots via props** — when `children` isn't enough:

```jsx
function SplitPane(props) {
  return (
    <div className="SplitPane">
      <div className="SplitPane-left">{props.left}</div>
      <div className="SplitPane-right">{props.right}</div>
    </div>
  );
}
```

> "React elements are just objects, so you can pass them as props like any other data. This approach may remind you of 'slots' in other libraries but there are no limitations on what you can pass as props in React." — React legacy docs

**Specialization** is not subclassing — it's a specific component rendering a generic one with preset props.

Composition is also a **performance tool**. Lifting content up as `children` means a parent re-render doesn't re-render the subtree it just passes through.

> "When the `color` changes, `ColorPicker` re-renders. But it still has the same `children` prop it got from the `App` last time, so React doesn't visit that subtree." — Dan Abramov, *Before You memo()*

> "Using the `children` prop to split up components usually makes the data flow of your application easier to follow and reduces the number of props plumbed down through the tree. Improved performance in cases like this is a cherry on top, not the end goal." — Abramov

---

## 2. Custom hooks — the primary abstraction

Custom hooks are how you share **stateful logic** in React. Not state — logic.

> "When you extract logic into custom Hooks, you can hide the gnarly details of how you deal with some external system or a browser API. The code of your components expresses your intent, not the implementation." — React docs, *Reusing Logic with Custom Hooks*

> "Hook names must start with `use` followed by a capital letter… This convention guarantees that you can always look at a component and know where its state, Effects, and other React features might 'hide'." — React docs

> "If your function doesn't call any Hooks, avoid the `use` prefix." — React docs

### Custom hooks share logic, not state

> "Custom Hooks let you share stateful logic but not state itself. Each call to a Hook is completely independent from every other call to the same Hook." — React docs

> "When you need to share the state itself between multiple components, lift it up and pass it down instead." — React docs

### Don't extract prematurely

> "You don't need to extract a custom Hook for every little duplicated bit of code. Some duplication is fine. For example, extracting a `useFormInput` Hook to wrap a single `useState` call like earlier is probably unnecessary." — React docs

### The name-as-intent test

> "Start by choosing your custom Hook's name. If you struggle to pick a clear name, it might mean that your Effect is too coupled to the rest of your component's logic, and is not yet ready to be extracted." — React docs

### Design hooks around concrete use cases, not around React's API

This is the big one. Don't make hooks that wrap `useEffect` in a slightly-different flavor. Make hooks that express a domain concept.

> "Keep custom Hooks focused on concrete high-level use cases. Avoid creating and using custom 'lifecycle' Hooks that act as alternatives and convenience wrappers for the `useEffect` API itself: 🔴 `useMount(fn)` 🔴 `useEffectOnce(fn)` 🔴 `useUpdateEffect(fn)`" — React docs

> "A good custom Hook makes the calling code more declarative by constraining what it does. For example, `useChatRoom(options)` can only connect to the chat room, while `useImpressionLog(eventName, extraData)` can only send an impression log to the analytics. If your custom Hook API doesn't constrain the use cases and is very abstract, in the long run it's likely to introduce more problems than it solves." — React docs

### Hooks wrap effects for a reason

> "Whenever you write an Effect, consider whether it would be clearer to also wrap it in a custom Hook. You shouldn't need Effects very often, so if you're writing one, it means that you need to 'step outside React' to synchronize with some external system." — React docs

---

## 3. Compound components

Multiple sibling components that cooperate via implicit shared state — modeled on HTML's `<select>`/`<option>`.

> "The idea is that you have two or more components that work together to accomplish a useful task. Typically one component is the parent, and the other is the child. The objective is to provide a more expressive and flexible API." — Kent C. Dodds

> "Think of it like `<select>` and `<option>`… If you were to try and use one without the other it wouldn't work (or make sense). Additionally it's actually a really great API." — Kent C. Dodds

**Implicit state** is the key. State lives on the parent; children read it via Context without the consumer having to thread props.

> "The `<select>` element implicitly stores state about the selected option and shares that with it's children so they know how to render themselves based on that state." — Kent C. Dodds

### The pattern, minimal implementation

```js
const ToggleContext = React.createContext()

function Toggle(props) {
  const [on, setOn] = React.useState(false)
  const toggle = React.useCallback(() => setOn((oldOn) => !oldOn), [])
  const value = React.useMemo(() => ({ on, toggle }), [on])
  return (
    <ToggleContext.Provider value={value}>
      {props.children}
    </ToggleContext.Provider>
  )
}

function useToggleContext() {
  const context = React.useContext(ToggleContext)
  if (!context) {
    throw new Error(
      `Toggle compound components cannot be rendered outside the Toggle component`,
    )
  }
  return context
}
```

The **guard hook** (`useXContext` that throws outside its provider) is non-negotiable. It turns "undefined is not an object" into a clear dev-time error.

### The consumer API

```jsx
<Menu>
  <MenuButton>Actions <span aria-hidden>▾</span></MenuButton>
  <MenuList>
    <MenuItem onSelect={() => alert('Download')}>Download</MenuItem>
    <MenuItem onSelect={() => alert('Copy')}>Create a Copy</MenuItem>
    <MenuItem onSelect={() => alert('Delete')}>Delete</MenuItem>
  </MenuList>
</Menu>
```

---

## 4. Render props and prop getters

Before hooks, the way you shared logic without opinionating the UI was **render props**. Today hooks are better at that specific job, but render props still have a place for slot-like composition.

> "The basic idea of the pattern is that rather than have the toggle component be responsible for doing anything special in the render method, we delegate that responsibility over to the user and we give them the state and functions necessary to allow the user of the component to render what they need for their use case." — Kent C. Dodds

> "Today however, we have React Hooks and hooks are way better at doing this than render props." — Kent C. Dodds

**Prop collections** are pre-bundled prop objects consumers spread onto an element. Leaky — they break when two handlers need to compose.

**Prop getters** are the fix: a function that *merges* consumer props with library props.

> "It's basically a function which will return props when called and people must apply those props to the right element to hook together all the relevant elements to make the overarching component." — Kent C. Dodds

```js
const callAll =
  (...fns) =>
  (...args) =>
    fns.forEach((fn) => fn && fn(...args))

getTogglerProps = (props = {}) => ({
  'aria-controls': 'target',
  'aria-expanded': Boolean(this.getOn()),
  ...props,
  onClick: callAll(props.onClick, this.toggle),
})
```

`callAll` is the pattern to steal — it's how you combine a consumer's `onClick` with your own without stepping on them.

---

## 5. State reducers and inversion of control

When a component's options list is growing (`closeOnSelect`, `resetOnOpen`, `preventAutoClose`…), stop adding options. Hand control back to the consumer via a reducer.

> "The benefit of the state reducer pattern is in the fact that it allows 'inversion of control' which is basically a mechanism for the author of the API to allow the user of the API to control how things work internally." — Kent C. Dodds

> "End user does an action / Dev calls dispatch / Hook determines the necessary changes / Hook calls dev's code for further changes 👈 this is the inversion of control part / Hook makes the state changes" — Kent C. Dodds

The meta-principle:

> "You can think of it as this: 'Make your abstraction do less stuff, and make your users do that instead.'" — Kent C. Dodds, *Inversion of Control*

### Name initial-only props clearly

> "In the rare case that this behavior is intentional, make sure to call that prop `initialColor` or `defaultColor` to clarify that changes to it are ignored." — Dan Abramov, *Writing Resilient Components*

---

## 6. AHA — Avoid Hasty Abstractions

DRY is a trap when the shape isn't settled. The alternative:

> "AHA (pronounced 'Aha!' like you just made a discovery) is an acronym I got from Cher Scarlett which stands for Avoid Hasty Abstractions" — Kent C. Dodds

> "prefer duplication over the wrong abstraction" — Sandi Metz, quoted by Kent

> "duplication is far cheaper than the wrong abstraction" — Sandi Metz, *The Wrong Abstraction*

> "Optimize for change first" — Kent C. Dodds

### The rule

> "I'm fine with code duplication until you feel pretty confident that you know the use cases for that duplicate code. What parts of the code are different that would make good arguments to your function? After you've got a few places where that code is running, the commonalities will scream at you for abstraction and you'll be in the right frame of mind to provide that abstraction." — Kent C. Dodds

### The failure mode

> "If you abstract early though, you'll think the function or component is perfect for your use case and so you just bend the code to fit your new use case. This goes on several times until the abstraction is basically your whole application in if statements and loops 😂😭" — Kent C. Dodds

### The takeaway

> "You shouldn't be dogmatic about when you start writing abstractions but instead write the abstraction when it feels right and don't be afraid to duplicate code until you get there." — Kent C. Dodds

> "Breaking a single component into multiple components is what's called 'abstraction.' Abstraction is awesome, but every abstraction comes with a cost, and you have to be aware of that cost and the benefits before you take the plunge." — Kent C. Dodds

> "It's WAY easier to maintain it until it needs to be broken up than maintain a pre-mature abstraction." — Kent C. Dodds

Dan Abramov's *The WET Codebase* (Deconstruct 2019) is the longer treatment:

> "Just because the structure of these two snippets looks similar, it might just mean that you don't really understand the problem yet" — Abramov

> "abstraction creates accidental coupling" — Abramov

> "we create this lasagna code where there are so many layers" — Abramov

> "abstraction also creates inertia in your code base" — Abramov

> "does your technology make it easier for you to get rid of them?" — Abramov's framework for evaluating abstractions

---

## 7. Thinking in React — decomposition heuristics

> "When you build a user interface with React, you will first break it apart into pieces called components. Then, you will describe the different visual states for each of your components. Finally, you will connect your components together so that the data flows through them." — React docs, *Thinking in React*

> "Start by drawing boxes around every component and subcomponent in the mockup and naming them." — React docs

> "If your JSON is well-structured, you'll often find that it naturally maps to the component structure of your UI. That's because UI and data models often have the same information architecture." — React docs

### State triage

> "Think of state as the minimal set of changing data that your app needs to remember." — React docs

> "Does it remain unchanged over time? If so, it isn't state. Is it passed in from a parent via props? If so, it isn't state. Can you compute it based on existing state or props in your component? If so, it definitely isn't state!" — React docs

### When NOT to break up a component

> "When you experience one of the problems above, that's when you break your component into multiple smaller components. NOT BEFORE." — Kent C. Dodds, *When to break up a component*

> "So I don't mind if the JSX I return in my component function gets really long… it's much easier to keep that code as it is than breaking out things into a bunch of smaller components and start Prop Drilling everywhere." — Kent C. Dodds

> "The key here is explicitness over implicitness." — Kent C. Dodds, *Prop Drilling*

Decomposition is **problem-driven, not elegance-driven**.

---

## 8. Presentational vs Container — and the retraction

The 2019 retraction is what matters:

> "Update from 2019: I wrote this article a long time ago and my views have since evolved. In particular, I don't suggest splitting your components like this anymore. If you find it natural in your codebase, this pattern can be handy. But I've seen it enforced without any necessity and with almost dogmatic fervor far too many times. The main reason I found it useful was because it let me separate complex stateful logic from other aspects of the component. Hooks let me do the same thing without an arbitrary division. This text is left intact for historical reasons but don't take it too seriously." — Dan Abramov

The durable idea from the 2015 original:

> "Remember, components don't have to emit DOM. They only need to provide composition boundaries between UI concerns. Take advantage of that." — Abramov

The takeaway: components are composition boundaries. Whether that seam is "container vs presentational" or "hook vs view" depends on the codebase. Don't enforce a split dogmatically.

---

## 9. The four Resilient Components principles

From Abramov's *Writing Resilient Components*:

1. **Don't stop the data flow.**
2. **Always be ready to render.**
3. **No component is a singleton.**
4. **Keep the local state isolated.**

Supporting quotes:

> "No amount of indentation or sorting imports alphabetically can fix a broken design. So instead of focusing on how some code looks, I will focus on how it works." — Abramov

> "When somebody uses your component, they expect that they can pass different props to it over time, and that the component will reflect those changes." — Abramov

> "A common mistake when learning React is to copy props into state." — Abramov

> "Props and state are a part of the React data flow. Both rendering and side effects should reflect changes in that data flow, not ignore them!" — Abramov

### Markbåge's axis of tension

> "Our approach really stems from making code consistent. This sometimes means making it equally confusing in all cases rather than easy in common ones. The theory is that if you have one way of doing it, you prepare your mental model for dealing with the hard problems. Even if it takes some getting used to up front." — Markbåge

Markbåge's principle — **consistency over common-case ergonomics** — is why hooks return stable shapes and why composition beats auto-reactivity.

---

## Sources

1. https://kentcdodds.com/blog/aha-programming
2. https://kentcdodds.com/blog/when-to-break-up-a-component-into-multiple-components
3. https://kentcdodds.com/blog/compound-components-with-react-hooks
4. https://kentcdodds.com/blog/advanced-react-component-patterns
5. https://kentcdodds.com/blog/how-to-give-rendering-control-to-users-with-prop-getters
6. https://kentcdodds.com/blog/the-state-reducer-pattern-with-react-hooks
7. https://kentcdodds.com/blog/inversion-of-control
8. https://kentcdodds.com/blog/how-to-use-react-context-effectively
9. https://kentcdodds.com/blog/prop-drilling
10. https://overreacted.io/writing-resilient-components/
11. https://medium.com/@dan_abramov/smart-and-dumb-components-7ca2f9a7c7d0
12. https://www.deconstructconf.com/2019/dan-abramov-the-wet-codebase
13. https://sandimetz.com/blog/2016/1/20/the-wrong-abstraction
14. https://react.dev/learn/thinking-in-react
15. https://react.dev/learn/reusing-logic-with-custom-hooks
16. https://react.dev/learn/you-might-not-need-an-effect
17. https://react.dev/learn/passing-props-to-a-component
18. https://legacy.reactjs.org/docs/composition-vs-inheritance.html
19. https://react.dev/reference/react/Children
20. https://gist.github.com/sebmarkbage/a5ef436427437a98408672108df01919
21. https://www.youtube.com/watch?v=3XaXKiXtNjw
22. https://egghead.io/courses/advanced-react-component-patterns
