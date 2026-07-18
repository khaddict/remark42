export const OAUTH_DATA = {
  custom: require('assets/social/custom.svg').default as string,
  github: {
    name: 'GitHub',
    icons: {
      light: require('assets/social/github.svg').default as string,
      dark: require('assets/social/github.svg').default as string,
    },
  },
} as const;
