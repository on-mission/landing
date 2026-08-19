---
title: "State and Data — Colocation, Server State, and the Modern Consensus"
summary: >
  How to think about state in a React app: server state vs client state, colocation, lifting,
  URL as state, data loading, when Redux still makes sense and when it doesn't, and why Context
  is not a state management tool. Built from Kent C. Dodds' colocation posts, Dan Abramov's
  "You Might Not Need Redux", Tanner Linsley and TkDodo on React Query, Ryan Florence's Remix
  philosophy, and Mark Erikson's measured Redux-maintainer perspective.
source_count: 29
---

# State and Data — Colocation, Server State, and the Modern Consensus

> "every type of state can fall into one of two buckets: Server Cache or UI State. We make a mistake when we combine the two." — Kent C. Dodds

> "People often choose Redux before they need it." — Dan Abramov

---

## 1. React is already a state management library

Before reaching for a library:

> "if you're building an application with React, you already have a state management library installed in your application." — Kent C. Dodds

> "React is a state management library" — Kent C. Dodds

> "Managing state is arguably the hardest part of any application. One of the things that makes it so difficult is that we often over-engineer our solution to the problem." — Kent C. Dodds

> "think of how your application's state maps to the application's tree structure." — Kent C. Dodds

---

## 2. Server state vs client state

The most important framing shift in React state management in the last decade. There are two kinds of state and they have different problems.

**Server state** (or "server cache"): data that lives authoritative somewhere else, arrives async, can go stale, is shared with other users.

**Client/UI state**: ephemeral local state — form drafts, modal open/closed, selected tab.

> "every type of state can fall into one of two buckets: Server Cache or UI State" — Kent C. Dodds

> "We make a mistake when we combine the two." — Kent C. Dodds

> "caching is a really hard problem (some say that it's one of the hardest in computer science)" — Kent C. Dodds

Tanner Linsley's framing:

> "fetching, caching, synchronizing and updating server state in your web applications" — TanStack Query

> "server state is totally different" — TanStack Query

**Server state is:**
- "Persisted remotely in a location you may not control or own"
- "Requires asynchronous APIs for fetching and updating"
- "Implies shared ownership and can be changed by other people without your knowledge"
- "Can potentially become 'out of date' in your applications if you're not careful"

> "Server state has its own unique challenges that we never face with client state" — Tanner Linsley

> "Global state is super convenient. It helps us avoid prop drilling" — Tanner Linsley

> "I think we've made a really big mistake by placing it there" — Tanner Linsley, on putting server data into global client state

TkDodo's re-framing:

> "React Query is in fact NOT a data fetching library." — TkDodo

> "React Query is an async state manager. It can manage any form of asynchronous state." — TkDodo

> "React Query manages async state (or, in terms of data fetching: server state), it assumes that the frontend application doesn't 'own' the data." — TkDodo

> "It is the server who owns the data." — TkDodo

> "If you get data from `useQuery`, try not to put that data into local state." — TkDodo

> "Stale data is better than no data, because no data usually means a loading spinner." — TkDodo

---

## 3. Colocation — keep state close to where it's used

> "Place code as close to where it's relevant as possible" — Kent C. Dodds, *Colocation*

> "Things that change together should be located as close as reasonable." — Kent C. Dodds

> "Localizing state has even more benefits than maintainability, it also improves performance" — Kent C. Dodds

### Why colocation is a performance win

> "the best way to make something fast is to do less stuff" — Kent C. Dodds, *State Colocation Will Make Your React App Faster*

> "If you move your state further down the React tree, then React has less to check." — Kent C. Dodds

> "Keep your state as close to where it's used as possible, and you'll benefit from a maintenance perspective and a performance perspective." — Kent C. Dodds

> "Colocate your state and you'll find yourself with a faster, simpler codebase." — Kent C. Dodds

> "Ask yourself 'do I really need the modal's `status` (open/closed) state to be in Redux?'" — Kent C. Dodds

> "Not everything in your application needs to be in a single state object." — Kent C. Dodds

### Abramov's version

> "Verify that you didn't put the state higher in the tree than necessary." — Dan Abramov, *Before You memo()*

> "Before you apply optimizations like `memo` or `useMemo`, it might make sense to look if you can split the parts that change from the parts that don't change." — Abramov

**The rule:** start with state as local as possible. Move it up only when you have a concrete reason. Move it back down when that reason goes away.

---

## 4. Lifting state up

> "Sometimes, you want the state of two components to always change together. To do it, remove state from both of them, move it to their closest common parent, and then pass it down to them via props. This is known as lifting state up." — React docs

> "For each unique piece of state, you will choose the component that 'owns' it. This principle is also known as having a 'single source of truth'" — React docs

> "It is common that you will move state down or back up while you're still figuring out where each piece of the state 'lives'. This is all part of the process!" — React docs

> "'Lifting State Up' is legitimately the answer to the state management problem in React and it's a rock solid one." — Kent C. Dodds

---

## 5. URL as state — the Remix thesis

A huge chunk of what developers put into React state belongs in the URL: filters, search queries, pagination, selected tabs, sort order, which record is open.

> "Instead of synchronizing state, you can read and set the state in the URL directly with boring old HTML forms." — Remix docs

> "State comes from a single source of truth without any state synchronization required." — Remix docs

> "The form is functional even before JavaScript loads." — Remix docs

> "A lot of data that developers might be tempted to store in React state has a more natural home in Remix." — Remix docs

> "If your React state is managing anything related to the network...it's likely that you're managing state that Remix already manages." — Remix docs

> "The primary criterion when choosing among these tools is whether you want the URL to change or not" — Remix docs, *Form vs. Fetcher*

The web has had state management since 1993. It's the URL. If your "state" should survive a refresh, a share, a back button — it belongs there.

---

## 6. The case against Redux-for-everything (and when Redux still wins)

The famous 2016 essay:

> "People often choose Redux before they need it." — Dan Abramov, *You Might Not Need Redux*

> "Redux offers a tradeoff." — Abramov

> "The tradeoff that Redux offers is to add indirection to decouple what happened from how things change." — Abramov

> "Is it always a good thing to do? No. It's a tradeoff." — Abramov

> "If you're just learning React, don't make Redux your first choice." — Abramov

> "Local state is fine." — Abramov

> "Come back to Redux if you find a real need for it." — Abramov

> "don't use Redux until you have problems with vanilla React" — Abramov

Kent's take:

> "I consistently see developers putting all of their state into redux. Not just global application state, but local state as well." — Kent C. Dodds

> "the ubiquity of redux is because it solved the prop drilling pain point for developers." — Kent C. Dodds

### Mark Erikson's measured defense

Mark maintains Redux and his writing is the adult-in-the-room counterweight:

> "you should always try to understand what problems a specific tool is trying to solve" — Erikson

> "pick the tools that solve your problem best. not because someone else said you should use them" — Erikson

> "if the only thing you needed to do with Redux is avoid passing data as props through 15 levels of your components - well, that's literally what Context was invented to do." — Erikson

> "Redux is a very generic state management tool that can be used for a broad array of use cases. It might not be quite the best at all of them, but you can do lots of different things." — Erikson

**When Redux still wins** (Erikson):

> "there's still many excellent reasons to choose Redux: Consistent architectural patterns, Debugging capabilities, Middleware, Addons and extensibility" — Erikson

> "The patterns and tools provided by Redux make it easier to understand when, where, why, and how the state in your application is being updated." — Erikson

### Redux docs' own guidance

> "Redux is most useful in cases when you have large amounts of application state that are needed in many places in the app, the app state is updated frequently, the logic to update that state may be complex, the app has a medium or large-sized codebase, and might be worked on by many people, or you need to see how that state is being updated over time." — Redux FAQ

> "Not all apps need Redux." — Redux FAQ

> "You'll know when you need Flux. If you aren't sure if you need it, you don't need it." — Pete Hunt, quoted in Redux FAQ

**Redux's own answer to server state:**

> "We specifically recommend using RTK Query for data fetching." — Redux docs

> "RTK Query replaces the need to write any actions, thunks, reducers, selectors, or effects to manage data fetching." — Redux docs

---

## 7. Data loading — loaders, actions, revalidation

The Remix/React Router pattern:

> "One of the primary features of Remix is the way it automatically keeps your UI in sync with persistent server state." — Remix docs

> "Route loaders provide data to the UI. Forms post data to route actions that update persistent state. Loader data on the page is automatically revalidated." — Remix docs

> "After the action completes, loaders are revalidated to get the new server state." — Remix docs

> "In this way, the UI is kept in sync with server state without writing any code for that synchronization." — Remix docs

> "UI is a function of your remote state and your local state." — Remix blog

> "Remote state is any data that needs to persist, like user data" — Remix blog

> "local state is ephemeral data which can be lost (e.g. via a refresh) without negatively impacting the user experience" — Remix blog

> "Loading data in only one way, on the server, leads to cleaner abstractions." — Remix blog

### Fetch-on-route, not fetch-on-render

The framework knows which route is loading, so it can start data loading *before* components render. Kills the waterfalls that pure-React apps create when each component fetches its own data in `useEffect`.

---

## 8. Context is not state management

> "Context lets the parent component make some information available to any component in the tree below it—no matter how deep—without passing it explicitly through props." — React docs

> "Just because you need to pass some props several levels deep doesn't mean you should put that information into context." — React docs

> "Start by passing props. If your components are not trivial, it's not unusual to pass a dozen props down through a dozen components. It may feel like a slog, but it makes it very clear which components use which data!" — React docs

> "Extract components and pass JSX as children to them. If you pass some data through many layers of intermediate components that don't use that data (and only pass it further down), this often means that you forgot to extract some components along the way." — React docs

Kent's rules:

> "you shouldn't be reaching for context to solve every state sharing problem that crosses your desk" — Kent C. Dodds

> "context does NOT have to be global to the whole app, but can be applied to one part of your tree" — Kent C. Dodds

> "you can (and probably should) have multiple logically separated contexts in your app" — Kent C. Dodds

> "99% of the time that you're going to be creating and using context in your application, you want your context consumers to be rendered within a provider" — Kent C. Dodds

Mark Erikson's sharpest framing — this settles a recurring argument:

> "Context is a form of Dependency Injection. It is a transport mechanism - it doesn't 'manage' anything." — Erikson

> "State management is how state changes over time." — Erikson

> "Context is how state (that exists somewhere already) is shared with other components." — Erikson

> "React Context does not meet those criteria. Therefore, Context is not a 'state management' tool!" — Erikson

> "These are different tools that solve different problems!" — Erikson

> "My personal opinion is that if you get past 2-3 state-related contexts in an application, you're re-inventing a weaker version of React-Redux." — Erikson

> "React-Redux only passes down the Redux store instance via context, not the current state value!" — Erikson

**The bright line:** Context = transport. State management = something that stores state and tells subscribers when it changes.

---

## 9. Forms as mutations

> "Data mutations are modeled as HTML forms." — Remix blog

> "A mutation is modeled as a form and a server page to handle it." — Remix blog

> "It posts with `fetch` instead of a document reload and then revalidates all the data on the page." — Remix blog

> "There's no application code needed to communicate a mutation with the server other than the form and the serverside action." — Remix blog

> "Instead of inventing another new JavaScript request/response API, Remix uses the Web Fetch API." — Remix blog

> "Get better at Remix, accidentally get better at the web." — Remix blog

> "When you learn how to handle requests and send responses in Remix, you're actually learning the Web Fetch API that's in the browser already." — Ryan Florence

### useReducer — React's own answer before reaching for a library

> "Components with many state updates spread across many event handlers can get overwhelming. For these cases, you can consolidate all the state update logic outside your component in a single function, called a reducer." — React docs

> "Reducers must be pure. Similar to state updater functions, reducers run during rendering!" — React docs

> "We recommend using a reducer if you often encounter bugs due to incorrect state updates in some component, and want to introduce more structure to its code." — React docs

---

## Sources

1. https://kentcdodds.com/blog/application-state-management-with-react
2. https://kentcdodds.com/blog/colocation
3. https://kentcdodds.com/blog/state-colocation-will-make-your-react-app-faster
4. https://kentcdodds.com/blog/how-to-use-react-context-effectively
5. https://kentcdodds.com/blog/prop-drilling
6. https://medium.com/@dan_abramov/you-might-not-need-redux-be46360cf367
7. https://twitter.com/dan_abramov/status/699241546248536064
8. https://overreacted.io/before-you-memo/
9. https://react.dev/learn/sharing-state-between-components
10. https://react.dev/learn/passing-data-deeply-with-context
11. https://react.dev/learn/extracting-state-logic-into-a-reducer
12. https://tanstack.com/query/latest/docs/framework/react/overview
13. https://gitnation.com/contents/react-query-its-time-to-break-up-with-your-global-state
14. https://tkdodo.eu/blog/react-query-as-a-state-manager
15. https://tkdodo.eu/blog/practical-react-query
16. https://blog.isquaredsoftware.com/2021/01/context-redux-differences/
17. https://blog.isquaredsoftware.com/2018/03/redux-not-dead-yet/
18. https://changelog.com/posts/when-and-when-not-to-reach-for-redux
19. https://redux.js.org/faq/general
20. https://redux.js.org/tutorials/essentials/part-1-overview-concepts
21. https://redux.js.org/usage/migrating-to-modern-redux
22. https://remix.run/docs/en/main/discussion/data-flow
23. https://remix.run/docs/en/main/discussion/state-management
24. https://remix.run/docs/en/main/discussion/form-vs-fetcher
25. https://remix.run/blog/remix-data-flow
26. https://remix.run/blog/not-another-framework
27. https://remix.run/blog/remix-vs-next
28. https://www.youtube.com/watch?v=95B8mnhzoCM
29. https://blog.isquaredsoftware.com/2024/07/presentations-why-use-redux/
