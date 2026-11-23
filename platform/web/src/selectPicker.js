export function installSelectPicker() {
  if (CSS.supports('appearance', 'base-select') && CSS.supports('selector(::picker(select))')) return
  if (!HTMLElement.prototype.showPopover) return

  const picker = document.createElement('div')
  picker.className = 'select-picker'
  picker.popover = 'manual'
  picker.id = 'workbench-select-picker'
  picker.setAttribute('role', 'listbox')
  picker.setAttribute('aria-label', '选项')

  let select = null
  let active = -1
  let search = ''
  let lastSearch = 0

  function close() {
    if (!select) return
    picker.hidePopover()
    select.removeAttribute('aria-expanded')
    select.removeAttribute('aria-controls')
    select.removeAttribute('aria-activedescendant')
    select = null
  }

  function enabled(index) {
    return picker.children[index]?.getAttribute('aria-disabled') === 'false'
  }

  function highlight(index) {
    if (!enabled(index)) return
    if (active >= 0) picker.children[active].classList.remove('keyboard-active')
    active = index
    const item = picker.children[index]
    item.classList.add('keyboard-active')
    select.setAttribute('aria-activedescendant', item.id)
    scrollToItem(item)
  }

  function scrollToItem(item) {
    const bounds = item.getBoundingClientRect()
    const frame = picker.getBoundingClientRect()
    if (bounds.bottom > frame.bottom - 4) picker.scrollTop += bounds.bottom - frame.bottom + 4
    else if (bounds.top < frame.top + 4) picker.scrollTop -= frame.top + 4 - bounds.top
  }

  function move(start, direction) {
    for (let index = start; index >= 0 && index < picker.childElementCount; index += direction) {
      if (enabled(index)) {
        highlight(index)
        return
      }
    }
  }

  function open(trigger, keyboard = false) {
    close()
    if (!trigger.options.length) return
    select = trigger
    active = -1
    search = ''
    lastSearch = 0
    select.focus({ preventScroll: true })
    select.setAttribute('aria-expanded', 'true')
    select.setAttribute('aria-controls', picker.id)
    picker.replaceChildren(...Array.from(select.options, (option, index) => {
      const item = document.createElement('div')
      item.id = `${picker.id}-${index}`
      item.dataset.index = index
      item.setAttribute('role', 'option')
      item.setAttribute('aria-selected', String(option.selected))
      item.setAttribute('aria-disabled', String(option.disabled || Boolean(option.closest('optgroup:disabled'))))
      item.textContent = option.label
      return item
    }))
    const bounds = select.getBoundingClientRect()
    const availableBelow = window.innerHeight - bounds.bottom - 8
    const availableAbove = bounds.top - 8
    const preferredHeight = Math.min(select.options.length * 34 + 10, 288)
    const above = availableBelow < preferredHeight && availableAbove > availableBelow
    Object.assign(picker.style, {
      left: `${bounds.left}px`,
      minWidth: `${bounds.width}px`,
      maxWidth: `${window.innerWidth - bounds.left - 8}px`,
      maxHeight: `${Math.min(288, above ? availableAbove : availableBelow)}px`,
      top: above ? 'auto' : `${bounds.bottom + 4}px`,
      bottom: above ? `${window.innerHeight - bounds.top + 4}px` : 'auto',
    })
    picker.classList.toggle('above', above)
    const container = select.closest('dialog[open]') || document.body
    container.append(picker)
    picker.showPopover()
    if (keyboard) {
      if (enabled(select.selectedIndex)) highlight(select.selectedIndex)
      else move(0, 1)
    } else if (select.selectedIndex >= 0) {
      scrollToItem(picker.children[select.selectedIndex])
    }
  }

  function commit(index) {
    if (!enabled(index) || !select?.isConnected || select.matches(':disabled')) return
    const control = select
    const changed = control.selectedIndex !== index
    control.selectedIndex = index
    close()
    if (changed) {
      control.dispatchEvent(new Event('input', { bubbles: true }))
      control.dispatchEvent(new Event('change', { bubbles: true }))
    }
  }

  function triggerFor(element) {
    const trigger = element.closest('select:enabled:not([multiple])')
    return trigger && trigger.size <= 1 ? trigger : null
  }

  document.addEventListener('mousedown', (event) => {
    if (picker.contains(event.target)) {
      event.preventDefault()
      return
    }
    const trigger = triggerFor(event.target)
    if (!trigger || event.button !== 0) {
      close()
      return
    }
    event.preventDefault()
  }, true)
  document.addEventListener('click', (event) => {
    const trigger = triggerFor(event.target)
    if (!trigger) return
    event.preventDefault()
    if (trigger === select) close()
    else open(trigger)
  }, true)
  picker.addEventListener('click', (event) => {
    const item = event.target.closest('[data-index]')
    if (item) commit(Number(item.dataset.index))
  })
  document.addEventListener('keydown', (event) => {
    const trigger = triggerFor(event.target)
    if (!trigger) return
    const { key } = event
    if (trigger !== select) {
      if (!['Enter', ' ', 'F4', 'ArrowDown', 'ArrowUp'].includes(key)) return
      event.preventDefault()
      open(trigger, true)
      return
    }
    if (key === 'Tab') {
      close()
      return
    }
    if (key === 'Escape') {
      event.preventDefault()
      event.stopPropagation()
      close()
    } else if (key === 'ArrowDown' || key === 'ArrowUp') {
      event.preventDefault()
      const direction = key === 'ArrowDown' ? 1 : -1
      move(active < 0 && direction < 0 ? picker.childElementCount - 1 : active + direction, direction)
    } else if (key === 'Home' || key === 'End') {
      event.preventDefault()
      move(key === 'Home' ? 0 : picker.childElementCount - 1, key === 'Home' ? 1 : -1)
    } else if (key === 'Enter' || key === ' ' || key === 'F4') {
      event.preventDefault()
      if (active >= 0 && key !== 'F4') commit(active)
      else close()
    } else if (key.length === 1 && !event.ctrlKey && !event.metaKey && !event.altKey) {
      event.preventDefault()
      search = performance.now() - lastSearch < 650 ? search + key.toLocaleLowerCase() : key.toLocaleLowerCase()
      lastSearch = performance.now()
      const index = Array.from(picker.children).findIndex((item, position) =>
        enabled(position) && item.textContent.toLocaleLowerCase().startsWith(search))
      if (index >= 0) highlight(index)
    }
  }, true)
  document.addEventListener('focusin', (event) => {
    if (select && event.target !== select) close()
  })
  window.addEventListener('resize', close)
  window.addEventListener('scroll', (event) => {
    if (event.target !== picker) close()
  }, true)
  document.addEventListener('change', (event) => {
    if (event.target === select) close()
  }, true)
}
