// Operations follow schema/schema.graphqls. The browser never calls the
// interpreter; every read and write goes through this GraphQL API.
import {
  Client,
  cacheExchange,
  fetchExchange,
  subscriptionExchange,
} from 'urql'
import { createClient as createWS } from 'graphql-ws'

const ws = createWS({
  url: () => {
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    return `${proto}//${window.location.host}/graphql`
  },
})

export const client = new Client({
  url: '/graphql',
  exchanges: [
    cacheExchange,
    subscriptionExchange({
      forwardSubscription(request) {
        return {
          subscribe(sink) {
            const unsubscribe = ws.subscribe(
              {
                query: request.query ?? '',
                variables: request.variables,
              },
              {
                next: (value) => {
                  // graphql-ws types extensions as unknown; urql wants its own ExecutionResult.
                  sink.next(value as never)
                },
                error: (err) => sink.error(err as Error),
                complete: () => sink.complete(),
              },
            )
            return { unsubscribe }
          },
        }
      },
    }),
    fetchExchange,
  ],
})

export const ModelsQuery = `
  query Models {
    models
  }
`

export const CreateSession = `
  mutation CreateSession($model: String!) {
    createSession(model: $model) {
      id
      modelName
      code
      notes
      widgets { index kind display forever }
    }
  }
`

export const LoadModel = `
  mutation LoadModel($id: ID!, $model: String!) {
    loadModel(sessionId: $id, model: $model) {
      id
      modelName
      code
      notes
      widgets { index kind display forever }
    }
  }
`

export const SetCode = `
  mutation SetCode($id: ID!, $code: String!) {
    setCode(sessionId: $id, code: $code) { ok error }
  }
`

export const RunCommand = `
  mutation RunCommand($id: ID!, $text: String!) {
    runCommand(sessionId: $id, text: $text) { ok output error }
  }
`

export const Setup = `
  mutation Setup($id: ID!) { setup(sessionId: $id) { id ticksStarted } }
`

export const Go = `
  mutation Go($id: ID!) { go(sessionId: $id) { id running } }
`

export const Stop = `
  mutation Stop($id: ID!) { stop(sessionId: $id) { id running } }
`

export const Step = `
  mutation Step($id: ID!) { step(sessionId: $id) { id ticks } }
`

export const PressButton = `
  mutation PressButton($id: ID!, $index: Int!) {
    pressButton(sessionId: $id, index: $index) { id running }
  }
`

export const SetSpeed = `
  mutation SetSpeed($id: ID!, $speed: Float!) {
    setSpeed(sessionId: $id, speed: $speed) { id speed }
  }
`

export const SetFrameRate = `
  mutation SetFrameRate($id: ID!, $fps: Int!) {
    setFrameRate(sessionId: $id, fps: $fps) { id frameRateCap }
  }
`

export const SetSlider = `
  mutation SetSlider($id: ID!, $index: Int!, $value: Float!) {
    setSlider(sessionId: $id, index: $index, value: $value) { id }
  }
`

export const SetSwitch = `
  mutation SetSwitch($id: ID!, $index: Int!, $on: Boolean!) {
    setSwitch(sessionId: $id, index: $index, on: $on) { id }
  }
`

export const SetChooser = `
  mutation SetChooser($id: ID!, $index: Int!, $current: Int!) {
    setChooser(sessionId: $id, index: $index, current: $current) { id }
  }
`

export const SetInput = `
  mutation SetInput($id: ID!, $index: Int!, $text: String!) {
    setInput(sessionId: $id, index: $index, text: $text) { id }
  }
`

export const WorldSettingsQuery = `
  query WorldSettings($id: ID!) {
    worldSettings(sessionId: $id) {
      minPxcor
      maxPxcor
      minPycor
      maxPycor
      patchSize
      wrapX
      wrapY
    }
  }
`

export const SetWorld = `
  mutation SetWorld(
    $id: ID!
    $minPxcor: Int!
    $maxPxcor: Int!
    $minPycor: Int!
    $maxPycor: Int!
    $patchSize: Float!
    $wrapX: Boolean!
    $wrapY: Boolean!
  ) {
    setWorld(
      sessionId: $id
      minPxcor: $minPxcor
      maxPxcor: $maxPxcor
      minPycor: $minPycor
      maxPycor: $maxPycor
      patchSize: $patchSize
      wrapX: $wrapX
      wrapY: $wrapY
    ) { id }
  }
`

export const Frames = `
  subscription Frames($id: ID!) {
    frames(sessionId: $id) {
      sessionId
      ticks
      ticksStarted
      tps
      timer
      frameRateCap
      droppedFrames
      droppedSlow
      publishedFps
      running
      speed
      log
      widgets {
        index
        kind
        display
        variable
        forever
        disabled
        min
        max
        value
        step
        units
        on
        reading
        choices
        current
        text
      }
      world {
        minPxcor
        maxPxcor
        minPycor
        maxPycor
        width
        height
        patchSize
        wrapX
        wrapY
        patchColors
        turtles { who x y heading size shape hidden r g b label }
        links { x1 y1 x2 y2 hidden r g b label }
        patchLabels { x y label }
      }
    }
  }
`
