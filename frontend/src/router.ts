import { createRouter, createWebHistory } from 'vue-router'
import MoviesView from './views/MoviesView.vue'
import MovieView from './views/MovieView.vue'
import ActressesView from './views/ActressesView.vue'
import ActressView from './views/ActressView.vue'
import AuthView from './views/AuthView.vue'
import SetupView from './views/SetupView.vue'
import AdminView from './views/AdminView.vue'
import PasswordView from './views/PasswordView.vue'
import AnnotationsView from './views/AnnotationsView.vue'
import RecommendationsView from './views/RecommendationsView.vue'

export const router = createRouter({
  history: createWebHistory(),
  scrollBehavior: () => ({ top: 0 }),
  routes: [
    { path: '/', component: MoviesView },
    { path: '/movies', component: MoviesView },
    { path: '/movies/:id', component: MovieView },
    { path: '/actresses', component: ActressesView },
    { path: '/actresses/name/:name', name: 'actress-by-name', component: ActressView },
    { path: '/actresses/:id', component: ActressView },
    { path: '/annotations', component: AnnotationsView },
    { path: '/recommendations', component: RecommendationsView },
    { path: '/login', component: AuthView },
    { path: '/register', component: AuthView },
    { path: '/forgot-password', component: PasswordView },
    { path: '/reset-password', component: PasswordView },
    { path: '/setup', component: SetupView },
    { path: '/admin/:section?', component: AdminView },
  ],
})
