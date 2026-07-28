// Masonry en orden de lectura.
//
// CSS `columns` reparte las tarjetas llenando cada columna de arriba a
// abajo, así que el segundo libro queda DEBAJO del primero. Aquí se
// reparten en el orden opuesto: cada tarjeta, en orden, va a la columna
// más corta en ese momento. Como todas empiezan vacías, las primeras
// tarjetas llenan la primera fila de izquierda a derecha, y a partir de
// ahí cada una tapa el hueco más alto — orden de lectura sin huecos.
//
// El número de columnas viene de la variable CSS --masonry-cols, que
// Tailwind cambia por breakpoint, así que el layout sigue siendo
// responsivo sin duplicar los puntos de corte aquí.
(function () {
  var grid = document.querySelector('[data-masonry]');
  if (!grid) return;

  var items = Array.prototype.slice.call(grid.children);
  if (!items.length) return;

  var columnClass = grid.getAttribute('data-masonry-column') || '';
  var fallbackClasses = (grid.getAttribute('data-masonry-fallback') || '').split(' ').filter(Boolean);
  var rendered = 0;

  function columnCount() {
    var value = getComputedStyle(grid).getPropertyValue('--masonry-cols');
    return parseInt(value, 10) || 1;
  }

  function layout() {
    var count = columnCount();
    if (count === rendered) return;
    rendered = count;

    // La primera vez se abandona el fallback de CSS columns.
    fallbackClasses.forEach(function (cls) {
      grid.classList.remove(cls);
    });
    grid.classList.add('flex', 'items-start', 'gap-4');

    var columns = [];
    grid.textContent = '';
    for (var i = 0; i < count; i++) {
      var column = document.createElement('div');
      column.className = 'flex min-w-0 flex-1 flex-col gap-4';
      grid.appendChild(column);
      columns.push({ el: column, height: 0 });
    }

    items.forEach(function (item) {
      var shortest = columns[0];
      for (var i = 1; i < columns.length; i++) {
        if (columns[i].height < shortest.height) shortest = columns[i];
      }
      shortest.el.appendChild(item);
      // offsetHeight fuerza el cálculo de layout, así la siguiente
      // tarjeta se coloca con la altura real de esta.
      shortest.height += item.offsetHeight;
    });
  }

  layout();

  var pending;
  window.addEventListener('resize', function () {
    clearTimeout(pending);
    pending = setTimeout(layout, 150);
  });
})();
