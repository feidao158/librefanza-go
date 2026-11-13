import { createRouter, createWebHistory } from 'vue-router'

const MoviesView = () => import('./views/MoviesView.vue')
const MovieView = () => import('./views/MovieView.vue')
const ActressesView = () => import('./views/ActressesView.vue')
const ActressView = () => import('./views/ActressView.vue')
const AuthView = () => import('./views/AuthView.vue')
const SetupView = () => import('./views/SetupView.vue')
const AdminView = () => import('./views/AdminView.vue')
const PasswordView = () => import('./views/PasswordView.vue')
const AnnotationsView = () => import('./views/AnnotationsView.vue')
const RecommendationsView = () => import('./views/RecommendationsView.vue')
const LikedView = () => import('./views/LikedView.vue')

export const router = createRouter({
  history: createWebHistory(),
  scrollBehavior: (to, from) => to.path === '/likes' && from.path.startsWith('/movies/') ? false : { top: 0 },
  routes: [
    { path: '/', component: MoviesView },
    { path: '/movies', component: MoviesView },
    { path: '/movies/:id', component: MovieView },
    { path: '/actresses', component: ActressesView },
    { path: '/actresses/name/:name', name: 'actress-by-name', component: ActressView },
    { path: '/actresses/:id', component: ActressView },
    { path: '/annotations', component: AnnotationsView },
    { path: '/recommendations', component: RecommendationsView },
    { path: '/likes', component: LikedView },
    { path: '/login', component: AuthView },
    { path: '/register', component: AuthView },
    { path: '/forgot-password', component: PasswordView },
    { path: '/reset-password', component: PasswordView },
    { path: '/setup', component: SetupView },
    { path: '/admin/:section?', component: AdminView },
  ],
})
